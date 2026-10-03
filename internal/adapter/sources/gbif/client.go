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

const (
	pageSize     = 300
	maxJSONBytes = 5 << 20
)

type Client struct {
	baseURL string
	limiter *rate.Limiter
	http    *http.Client
}

var _ ingest.OccurrenceSource = (*Client)(nil)

func New(baseURL string, limiter *rate.Limiter) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), limiter: limiter,
		http: &http.Client{Timeout: 20 * time.Second}}
}

type matchResponse struct {
	UsageKey  int    `json:"usageKey"`
	MatchType string `json:"matchType"`
}

type searchResponse struct {
	EndOfRecords bool `json:"endOfRecords"`
	Results      []struct {
		Lat float64 `json:"decimalLatitude"`
		Lng float64 `json:"decimalLongitude"`
	} `json:"results"`
}

func (c *Client) FindOccurrences(ctx context.Context, scientificName string, max int) ([]domain.Point, error) {
	var m matchResponse
	if err := c.getJSON(ctx, "/v1/species/match", url.Values{"name": {scientificName}}, &m); err != nil {
		return nil, err
	}
	if m.MatchType == "NONE" || m.UsageKey == 0 {
		return nil, nil // espécie desconhecida no GBIF não é erro
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
