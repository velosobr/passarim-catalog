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

const (
	brazilPlaceID = "6878" // identificador do Brasil no iNaturalist
	maxJSONBytes  = 5 << 20
	// Licenças que pedimos à fonte (ND nunca é pedida).
	licenses = "cc-by,cc-by-nc,cc-by-sa,cc-by-nc-sa,cc0"
)

type Client struct {
	baseURL   string
	userAgent string
	limiter   *rate.Limiter
	http      *http.Client
}

var _ ingest.PhotoSource = (*Client)(nil)

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

type observationsResponse struct {
	Results []struct {
		URI    string `json:"uri"`
		Photos []struct {
			ID          int    `json:"id"`
			URL         string `json:"url"`
			LicenseCode string `json:"license_code"`
			Attribution string `json:"attribution"`
			Dimensions  *struct {
				Width  int `json:"width"`
				Height int `json:"height"`
			} `json:"original_dimensions"`
		} `json:"photos"`
	} `json:"results"`
}

func (c *Client) FindPhotos(ctx context.Context, scientificName string, max int) ([]ingest.PhotoCandidate, error) {
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
		"per_page":      {strconv.Itoa(max)},
	}, &obs)
	if err != nil {
		return nil, err
	}
	var out []ingest.PhotoCandidate
	for _, o := range obs.Results {
		if len(o.Photos) == 0 {
			continue
		}
		p := o.Photos[0]
		cand := ingest.PhotoCandidate{
			SourceID: strconv.Itoa(p.ID),
			// A API devolve a miniatura quadrada; a versão grande tem o mesmo caminho.
			URL:     strings.Replace(p.URL, "square.", "large.", 1),
			PageURL: o.URI, License: p.LicenseCode, Author: p.Attribution,
		}
		if p.Dimensions != nil {
			cand.Width, cand.Height = p.Dimensions.Width, p.Dimensions.Height
		}
		out = append(out, cand)
	}
	return out, nil
}

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
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("inaturalist: status %d", resp.StatusCode)
	}
	// Limitamos o tamanho do JSON: resposta externa nunca é confiável (API10).
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(out); err != nil {
		return fmt.Errorf("inaturalist: resposta inválida: %w", err)
	}
	return nil
}
