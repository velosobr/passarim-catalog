// Package inaturalist busca fotos de observações no iNaturalist.
//
// Termos de uso: a API é aberta, mas pede um User-Agent que identifique o
// projeto e no máximo ~1 requisição por segundo. Só pedimos fotos com
// licenças Creative Commons que o Passarim aceita; o filtro final (por
// exemplo, NC desligado) é feito no caso de uso.
package inaturalist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// Valores fixos da busca no iNaturalist.
const (
	brazilPlaceID = "6878" // identificador do Brasil no iNaturalist
	maxJSONBytes  = 5 << 20
	// Licenças que pedimos à fonte (ND nunca é pedida).
	licenses = "cc-by,cc-by-nc,cc-by-sa,cc-by-nc-sa,cc0"
)

// Client é o cliente da API. O limiter garante o ritmo de no máximo uma
// requisição por segundo, e o http.Client tem prazo de 20 s por requisição.
type Client struct {
	baseURL   string
	userAgent string
	limiter   *rate.Limiter
	http      *http.Client
}

// Conferência em tempo de compilação: Client implementa ingest.PhotoSource.
var _ ingest.PhotoSource = (*Client)(nil)

// New cria o cliente; a barra final da URL base é removida.
func New(baseURL, userAgent string, limiter *rate.Limiter) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), userAgent: userAgent, limiter: limiter,
		http: &http.Client{Timeout: 20 * time.Second}}
}

// Só os campos que usamos: o resto da resposta é ignorado.
type taxaResponse struct {
	Results []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"results"`
}

// observation é a parte usada de uma observação: id, página, votos
// (faves_count, o campo que o order_by=votes usa), anotações e fotos.
type observation struct {
	ID          int          `json:"id"`
	URI         string       `json:"uri"`
	FavesCount  int          `json:"faves_count"`
	Annotations []annotation `json:"annotations"`
	Photos      []photo      `json:"photos"`
}

// annotation é uma anotação da comunidade ("está morto", "é uma pena"...).
// Os ids vêm de /v1/controlled_terms. vote_score é o saldo de votos: abaixo
// de 0, a comunidade discordou da anotação.
type annotation struct {
	AttributeID int `json:"controlled_attribute_id"`
	ValueID     int `json:"controlled_value_id"`
	VoteScore   int `json:"vote_score"`
}

// photo é uma foto de uma observação: id, URL, licença, autoria e dimensões.
type photo struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	LicenseCode string `json:"license_code"`
	Attribution string `json:"attribution"`
	Dimensions  *struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"original_dimensions"`
}

// observationsResponse é a resposta de /v1/observations (e de /v1/observations/{id}).
type observationsResponse struct {
	Results []observation `json:"results"`
}

// photosPerObservation: quantas fotos de cada observação viram candidatas.
// As primeiras costumam ser as melhores; a 4ª em diante quase sempre repete.
const photosPerObservation = 3

// rejectedAnnotations: termo -> valores que tiram a observação inteira.
// A busca da API não filtra isso direito (sem term_id o parâmetro é
// ignorado; com term_id ela também exclui quem não tem a anotação), por
// isso o filtro é feito aqui, sobre as anotações da resposta.
var rejectedAnnotations = map[int]map[int]bool{
	17: {19: true},                                         // Alive or Dead: Dead
	1:  {7: true},                                          // Life Stage: Egg
	22: {23: true, 27: true, 28: true, 30: true, 35: true}, // Evidence of Presence: Feather, Bone, Molt, Egg, Construction
}

// rejectedByAnnotation diz se alguma anotação, com saldo de votos >= 0,
// marca a observação como morto, ovo, pena, osso, muda ou ninho. A anotação
// vale para a observação inteira; o curador continua olhando foto por foto.
func rejectedByAnnotation(anns []annotation) bool {
	for _, a := range anns {
		if a.VoteScore >= 0 && rejectedAnnotations[a.AttributeID][a.ValueID] {
			return true
		}
	}
	return false
}

// candidate converte uma foto da observação em candidata. A API devolve a
// miniatura quadrada; a versão grande tem o mesmo caminho com "large.".
func candidate(o observation, p photo) ingest.PhotoCandidate {
	c := ingest.PhotoCandidate{
		SourceID: strconv.Itoa(p.ID), ObservationID: strconv.Itoa(o.ID), Votes: o.FavesCount,
		URL:     strings.Replace(p.URL, "square.", "large.", 1),
		PageURL: o.URI, License: p.LicenseCode, Author: p.Attribution,
	}
	if p.Dimensions != nil {
		c.Width, c.Height = p.Dimensions.Width, p.Dimensions.Height
	}
	return c
}

// errNotFound: a API respondeu 404.
var errNotFound = errors.New("inaturalist: não encontrado")

// FindPhotos busca o táxon pelo nome científico e depois as observações dele
// no Brasil: só "research grade" (identificação confirmada pela comunidade),
// só com as licenças aceitas e ordenadas pelos votos. De cada observação que
// passa no filtro de anotações, ficam até 3 fotos.
func (c *Client) FindPhotos(ctx context.Context, scientificName string, observations int) ([]ingest.PhotoCandidate, error) {
	var taxa taxaResponse
	if err := c.getJSON(ctx, "/v1/taxa", url.Values{"q": {scientificName}, "rank": {"species"}, "per_page": {"1"}}, &taxa); err != nil {
		return nil, err
	}
	// A busca de táxon é "autocomplete": o 1º resultado pode ser outra espécie
	// (taxonomias diferem). Só aceitamos se o nome científico bater.
	if len(taxa.Results) == 0 || !strings.EqualFold(taxa.Results[0].Name, scientificName) {
		return nil, nil // espécie desconhecida na fonte não é erro
	}
	var obs observationsResponse
	err := c.getJSON(ctx, "/v1/observations", url.Values{
		"taxon_id":      {strconv.Itoa(taxa.Results[0].ID)},
		"place_id":      {brazilPlaceID},
		"quality_grade": {"research"},
		"photo_license": {licenses},
		"order_by":      {"votes"},
		"per_page":      {strconv.Itoa(observations)},
	}, &obs)
	if err != nil {
		return nil, err
	}
	var out []ingest.PhotoCandidate
	for _, o := range obs.Results {
		if rejectedByAnnotation(o.Annotations) {
			continue
		}
		for _, p := range o.Photos[:min(len(o.Photos), photosPerObservation)] {
			out = append(out, candidate(o, p))
		}
	}
	return out, nil
}

// FindObservationPhoto busca a observação pelo id e procura a foto entre
// TODAS as dela (não só as 3 primeiras): é a principal escolhida à mão no
// YAML. A API não tem busca de foto pelo id. Observação inexistente (404)
// ou foto fora dela: ok=false, sem erro. O filtro de anotações não vale
// aqui: o dono escolheu essa foto olhando para ela.
func (c *Client) FindObservationPhoto(ctx context.Context, observationID, photoID string) (ingest.PhotoCandidate, bool, error) {
	var obs observationsResponse
	err := c.getJSON(ctx, "/v1/observations/"+url.PathEscape(observationID), url.Values{}, &obs)
	if errors.Is(err, errNotFound) {
		return ingest.PhotoCandidate{}, false, nil
	}
	if err != nil {
		return ingest.PhotoCandidate{}, false, err
	}
	for _, o := range obs.Results {
		for _, p := range o.Photos {
			if strconv.Itoa(p.ID) == photoID {
				return candidate(o, p), true, nil
			}
		}
	}
	return ingest.PhotoCandidate{}, false, nil
}

// getJSON faz um GET, espera a vez no rate limiter e decodifica o JSON.
func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	// Respeita o limite de requisições por segundo da fonte.
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("inaturalist: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("inaturalist: status %d", resp.StatusCode)
	}
	// Limitamos o tamanho do JSON: resposta externa nunca é confiável (API10).
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(out); err != nil {
		return fmt.Errorf("inaturalist: resposta inválida: %w", err)
	}
	return nil
}
