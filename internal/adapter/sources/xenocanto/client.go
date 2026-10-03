// Package xenocanto busca gravações de cantos no xeno-canto (API v3).
//
// A API exige uma chave pessoal (XENO_CANTO_API_KEY). Ela vai na URL da
// requisição, por isso NUNCA devolvemos o erro bruto do net/http (que cita
// a URL inteira): os erros daqui mencionam só o status ou um texto fixo.
package xenocanto

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

const maxJSONBytes = 5 << 20

type Client struct {
	baseURL   string
	apiKey    string
	userAgent string
	limiter   *rate.Limiter
	http      *http.Client
}

var _ ingest.AudioSource = (*Client)(nil)

func New(baseURL, apiKey, userAgent string, limiter *rate.Limiter) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, userAgent: userAgent, limiter: limiter,
		http: &http.Client{Timeout: 20 * time.Second}}
}

type response struct {
	Recordings []struct {
		ID     string `json:"id"`
		File   string `json:"file"`
		Lic    string `json:"lic"`
		Rec    string `json:"rec"`
		URL    string `json:"url"`
		Length string `json:"length"`
		Q      string `json:"q"`
		Type   string `json:"type"`
	} `json:"recordings"`
}

func (c *Client) FindRecordings(ctx context.Context, scientificName string) ([]ingest.AudioCandidate, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	q := url.Values{
		"query":    {fmt.Sprintf(`sp:"%s" cnt:brazil`, scientificName)},
		"key":      {c.apiKey},
		"per_page": {"50"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/3/recordings?"+q.Encode(), nil)
	if err != nil {
		return nil, errors.New("xenocanto: requisição inválida")
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		// Não embrulhamos err: o *url.Error contém a URL com a chave.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("xenocanto: falha de conexão")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xenocanto: status %d", resp.StatusCode)
	}
	var body response
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(&body); err != nil {
		return nil, errors.New("xenocanto: resposta inválida")
	}
	out := make([]ingest.AudioCandidate, 0, len(body.Recordings))
	for _, r := range body.Recordings {
		lic := r.Lic
		if strings.HasPrefix(lic, "//") { // a fonte às vezes omite o esquema
			lic = "https:" + lic
		}
		out = append(out, ingest.AudioCandidate{SourceID: r.ID, URL: r.File, PageURL: r.URL, License: lic, Author: r.Rec,
			Quality: r.Q, Type: r.Type, DurationMs: parseLength(r.Length)})
	}
	return out, nil
}

// parseLength converte "m:ss" (ex.: "1:26") em milissegundos; lixo vira 0.
func parseLength(s string) int {
	m, sec, ok := strings.Cut(s, ":")
	if !ok {
		return 0
	}
	min, err1 := strconv.Atoi(m)
	secs, err2 := strconv.Atoi(sec)
	if err1 != nil || err2 != nil || min < 0 || secs < 0 {
		return 0
	}
	return (min*60 + secs) * 1000
}
