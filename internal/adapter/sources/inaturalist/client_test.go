package inaturalist_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"github.com/velosobr/passarim-catalog/internal/adapter/sources/inaturalist"
)

func server(t *testing.T, taxaFile string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent ausente")
		}
		var file string
		switch r.URL.Path {
		case "/v1/taxa":
			file = taxaFile
		case "/v1/observations":
			q := r.URL.Query()
			if q.Get("place_id") != "6878" || q.Get("taxon_id") != "12738" || !strings.Contains(q.Get("photo_license"), "cc-by") {
				t.Errorf("parâmetros errados: %v", q)
			}
			file = "testdata/observations.json"
		default:
			http.NotFound(w, r)
			return
		}
		b, _ := os.ReadFile(file) //nolint:gosec // caminhos fixos de testdata
		_, _ = w.Write(b)
	}))
}

func TestFindPhotos(t *testing.T) {
	srv := server(t, "testdata/taxa.json")
	defer srv.Close()
	c := inaturalist.New(srv.URL, "test-agent", rate.NewLimiter(rate.Inf, 1))
	photos, err := c.FindPhotos(context.Background(), "Turdus rufiventris", 3)
	if err != nil || len(photos) == 0 {
		t.Fatalf("got %v %v", photos, err)
	}
	p := photos[0]
	if p.SourceID == "" || p.SourceID == "0" {
		t.Errorf("SourceID (id da foto no iNaturalist) ausente: %+v", p)
	}
	if !strings.Contains(p.URL, "/large.") || strings.Contains(p.URL, "square") {
		t.Errorf("URL deveria apontar para a versão large: %s", p.URL)
	}
	if p.License == "" || p.Author == "" || !strings.HasPrefix(p.PageURL, "https://www.inaturalist.org/observations/") || p.Width == 0 {
		t.Errorf("crédito incompleto: %+v", p)
	}
}

func TestFindPhotos_UnknownTaxonIsEmpty(t *testing.T) {
	srv := server(t, "testdata/taxa-empty.json")
	defer srv.Close()
	photos, err := inaturalist.New(srv.URL, "ua", rate.NewLimiter(rate.Inf, 1)).FindPhotos(context.Background(), "Nada nada", 3)
	if err != nil || len(photos) != 0 {
		t.Fatalf("táxon desconhecido: %v %v", photos, err)
	}
}

func TestFindPhotos_HTTPErrorIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	if _, err := inaturalist.New(srv.URL, "ua", rate.NewLimiter(rate.Inf, 1)).FindPhotos(context.Background(), "X y", 3); err == nil {
		t.Fatal("503 deveria virar erro")
	}
}

// Revisão final #4: a busca de táxon é "autocomplete"; o nome precisa bater.
func TestFindPhotos_TaxonWithDifferentNameIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/observations" {
			t.Error("não deveria buscar observações de outro táxon")
		}
		_, _ = w.Write([]byte(`{"results":[{"id":99,"name":"Turdus leucomelas","rank":"species"}]}`))
	}))
	defer srv.Close()
	photos, err := inaturalist.New(srv.URL, "ua", rate.NewLimiter(rate.Inf, 1)).FindPhotos(context.Background(), "Turdus rufiventris", 3)
	if err != nil || len(photos) != 0 {
		t.Fatalf("nome diferente deveria dar vazio: %v %v", photos, err)
	}
}
