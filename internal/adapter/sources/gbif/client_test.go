package gbif_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"

	"golang.org/x/time/rate"

	"github.com/velosobr/passarim-catalog/internal/adapter/sources/gbif"
)

func limiter() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

func TestFindOccurrences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var file string
		switch r.URL.Path {
		case "/v1/species/match":
			file = "testdata/match.json"
		case "/v1/occurrence/search":
			q := r.URL.Query()
			if q.Get("country") != "BR" || q.Get("hasCoordinate") != "true" || q.Get("hasGeospatialIssue") != "false" || q.Get("taxonKey") != "2490718" {
				t.Errorf("filtros errados: %v", q)
			}
			file = "testdata/occurrences.json"
		default:
			http.NotFound(w, r)
			return
		}
		b, _ := os.ReadFile(file)
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	pts, err := gbif.New(srv.URL, limiter()).FindOccurrences(context.Background(), "Turdus rufiventris", 900)
	if err != nil || len(pts) == 0 {
		t.Fatalf("got %v %v", pts, err)
	}
	if pts[0].Lat == 0 && pts[0].Lng == 0 {
		t.Errorf("ponto vazio: %+v", pts[0])
	}
}

func TestFindOccurrences_PaginatesUntilMax(t *testing.T) {
	var searches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/species/match" {
			_, _ = w.Write([]byte(`{"usageKey":1,"matchType":"EXACT"}`))
			return
		}
		searches.Add(1)
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		type rec struct {
			Lat float64 `json:"decimalLatitude"`
			Lng float64 `json:"decimalLongitude"`
		}
		results := make([]rec, 300)
		for i := range results {
			results[i] = rec{Lat: -10 - float64(offset+i)/1000, Lng: -50}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"offset": offset, "limit": 300, "endOfRecords": false, "results": results})
	}))
	defer srv.Close()
	pts, err := gbif.New(srv.URL, limiter()).FindOccurrences(context.Background(), "A b", 650)
	if err != nil || len(pts) != 650 {
		t.Fatalf("esperava 650 pontos, veio %d, %v", len(pts), err)
	}
	if searches.Load() != 3 {
		t.Fatalf("esperava 3 requisições de página, veio %d", searches.Load())
	}
}

func TestFindOccurrences_NoMatchIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/species/match" {
			t.Error(fmt.Sprintf("não deveria buscar ocorrências sem match: %s", r.URL))
		}
		b, _ := os.ReadFile("testdata/match-none.json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	pts, err := gbif.New(srv.URL, limiter()).FindOccurrences(context.Background(), "Nada nada", 900)
	if err != nil || len(pts) != 0 {
		t.Fatalf("sem match: %v %v", pts, err)
	}
}

func TestFindOccurrences_HTTPErrorIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	if _, err := gbif.New(srv.URL, limiter()).FindOccurrences(context.Background(), "A b", 10); err == nil {
		t.Fatal("500 deveria virar erro")
	}
}
