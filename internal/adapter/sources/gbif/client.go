// Package gbif busca avistamentos de aves no GBIF (Global Biodiversity
// Information Facility). A API é aberta e sem chave; usamos só ocorrências
// no Brasil, com coordenadas e sem problemas geoespaciais.
package gbif

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

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// pageSize é quantos avistamentos pedimos por página (o máximo do GBIF), e
// maxJSONBytes o maior JSON aceito (5 MB).
const (
	pageSize     = 300
	maxJSONBytes = 5 << 20
)

// Client é o cliente da API, com o ritmo controlado pelo limiter.
type Client struct {
	baseURL string
	limiter *rate.Limiter
	http    *http.Client
}

// Conferência em tempo de compilação: Client implementa ingest.OccurrenceSource.
var _ ingest.OccurrenceSource = (*Client)(nil)

// New cria o cliente; a barra final da URL base é removida.
func New(baseURL string, limiter *rate.Limiter) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), limiter: limiter,
		http: &http.Client{Timeout: 20 * time.Second}}
}

// matchResponse é a resposta de /v1/species/match: o id do táxon no GBIF e
// o tipo de correspondência (EXACT, FUZZY, HIGHERRANK...).
type matchResponse struct {
	UsageKey  int    `json:"usageKey"`
	MatchType string `json:"matchType"`
}

// searchResponse é uma página de avistamentos, com as coordenadas e o aviso
// de fim (endOfRecords).
type searchResponse struct {
	EndOfRecords bool `json:"endOfRecords"`
	Results      []struct {
		Lat float64 `json:"decimalLatitude"`
		Lng float64 `json:"decimalLongitude"`
	} `json:"results"`
}

// FindOccurrences acha o táxon pelo nome e pagina os avistamentos no Brasil
// até juntar max pontos ou acabarem as páginas.
func (c *Client) FindOccurrences(ctx context.Context, scientificName string, max int) ([]domain.Point, error) {
	var m matchResponse
	if err := c.getJSON(ctx, "/v1/species/match", url.Values{"name": {scientificName}, "class": {"Aves"}, "strict": {"true"}}, &m); err != nil {
		return nil, err
	}
	// Só EXACT: sem strict, o GBIF devolveria o GÊNERO (HIGHERRANK) quando não
	// acha a espécie, e FUZZY pode ser outra espécie de nome parecido. Com
	// strict=true ele já não sobe na classificação, mas conferimos mesmo assim
	// para o mapa nunca mostrar outra espécie. Desconhecida não é erro.
	if m.MatchType != "EXACT" || m.UsageKey == 0 {
		return nil, nil
	}
	var points []domain.Point
	for offset := 0; len(points) < max; offset += pageSize {
		var page searchResponse
		err := c.getJSON(ctx, "/v1/occurrence/search", url.Values{
			"taxonKey":           {strconv.Itoa(m.UsageKey)},
			"country":            {"BR"},
			"hasCoordinate":      {"true"},
			"hasGeospatialIssue": {"false"},
			"limit":              {strconv.Itoa(pageSize)},
			"offset":             {strconv.Itoa(offset)},
		}, &page)
		if err != nil {
			return nil, err
		}
		for _, r := range page.Results {
			if len(points) == max {
				break
			}
			points = append(points, domain.Point{Lat: r.Lat, Lng: r.Lng})
		}
		if page.EndOfRecords || len(page.Results) == 0 {
			break
		}
	}
	return points, nil
}

// getJSON faz um GET, espera a vez no rate limiter e decodifica o JSON.
func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Passarim/0.1 (+https://github.com/velosobr/passarim-docs)")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gbif: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gbif: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(out); err != nil {
		return fmt.Errorf("gbif: resposta inválida: %w", err)
	}
	return nil
}
