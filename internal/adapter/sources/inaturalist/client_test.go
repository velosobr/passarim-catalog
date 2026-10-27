// Testes do cliente do iNaturalist contra um servidor HTTP falso com os
// JSON de testdata/.

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

// server responde a busca de táxon com o arquivo dado e a de observações
// com observations.json.
func server(t *testing.T, taxaFile string) *httptest.Server {
	return serverWith(t, taxaFile, "testdata/observations.json")
}

// serverWith é o servidor falso com o arquivo de observações escolhido. Ele
// também responde GET /v1/observations/{id}: o id 1001 devolve
// observation-one.json, e qualquer outro dá 404.
func serverWith(t *testing.T, taxaFile, obsFile string) *httptest.Server {
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
			file = obsFile
		case "/v1/observations/1001":
			file = "testdata/observation-one.json"
		default:
			http.NotFound(w, r)
			return
		}
		b, _ := os.ReadFile(file) //nolint:gosec // caminhos fixos de testdata
		_, _ = w.Write(b)
	}))
}

// Cada observação vira uma candidata, com a URL da versão grande, a página,
// a licença e o autor.
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

// Táxon que não existe na fonte devolve lista vazia, sem erro.
func TestFindPhotos_UnknownTaxonIsEmpty(t *testing.T) {
	srv := server(t, "testdata/taxa-empty.json")
	defer srv.Close()
	photos, err := inaturalist.New(srv.URL, "ua", rate.NewLimiter(rate.Inf, 1)).FindPhotos(context.Background(), "Nada nada", 3)
	if err != nil || len(photos) != 0 {
		t.Fatalf("táxon desconhecido: %v %v", photos, err)
	}
}

// Um status de erro da API vira erro.
func TestFindPhotos_HTTPErrorIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	if _, err := inaturalist.New(srv.URL, "ua", rate.NewLimiter(rate.Inf, 1)).FindPhotos(context.Background(), "X y", 3); err == nil {
		t.Fatal("503 deveria virar erro")
	}
}

// A busca de táxon é "autocomplete"; o nome precisa bater.
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

// Até 3 fotos por observação, com a observação e os votos; a observação
// anotada como morta sai, e a sem foto não gera candidata.
func TestFindPhotos_CurationCandidates(t *testing.T) {
	srv := serverWith(t, "testdata/taxa.json", "testdata/observations-curation.json")
	defer srv.Close()
	c := inaturalist.New(srv.URL, "ua", rate.NewLimiter(rate.Inf, 1))
	photos, err := c.FindPhotos(context.Background(), "Turdus rufiventris", 20)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range photos {
		ids = append(ids, p.SourceID)
	}
	if strings.Join(ids, ",") != "11,12,13,31" {
		t.Fatalf("candidatas: %v (esperava 11,12,13 da obs 1001 e 31 da 1003)", ids)
	}
	if photos[0].ObservationID != "1001" || photos[0].Votes != 7 || photos[2].Votes != 7 || photos[3].Votes != 0 {
		t.Fatalf("observação ou votos errados: %+v", photos)
	}
}

// A busca pede 20 observações por página (o número que o caso de uso passa).
func TestFindPhotos_PerPageIsObservations(t *testing.T) {
	var perPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/taxa" {
			b, _ := os.ReadFile("testdata/taxa.json")
			_, _ = w.Write(b)
			return
		}
		perPage = r.URL.Query().Get("per_page")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	if _, err := inaturalist.New(srv.URL, "ua", rate.NewLimiter(rate.Inf, 1)).FindPhotos(context.Background(), "Turdus rufiventris", 20); err != nil {
		t.Fatal(err)
	}
	if perPage != "20" {
		t.Fatalf("per_page = %q, esperava 20", perPage)
	}
}

// FindObservationPhoto acha qualquer foto da observação (aqui a 5ª), com a
// URL grande e o crédito.
func TestFindObservationPhoto_Found(t *testing.T) {
	srv := server(t, "testdata/taxa.json")
	defer srv.Close()
	p, ok, err := inaturalist.New(srv.URL, "ua", rate.NewLimiter(rate.Inf, 1)).FindObservationPhoto(context.Background(), "1001", "15")
	if err != nil || !ok {
		t.Fatalf("esperava achar: %v %v", ok, err)
	}
	if p.SourceID != "15" || p.ObservationID != "1001" || p.License != "cc-by-sa" || p.Votes != 7 ||
		!strings.Contains(p.URL, "/15/large.") || p.PageURL != "https://www.inaturalist.org/observations/1001" || p.Width != 1600 {
		t.Fatalf("candidata do override: %+v", p)
	}
}

// Foto que não está na observação, ou observação que não existe (404): não
// achou, sem erro.
func TestFindObservationPhoto_NotFound(t *testing.T) {
	srv := server(t, "testdata/taxa.json")
	defer srv.Close()
	c := inaturalist.New(srv.URL, "ua", rate.NewLimiter(rate.Inf, 1))
	for _, ids := range [][2]string{{"1001", "99"}, {"404", "1"}} {
		if _, ok, err := c.FindObservationPhoto(context.Background(), ids[0], ids[1]); err != nil || ok {
			t.Fatalf("%v: esperava não achar sem erro, veio ok=%v err=%v", ids, ok, err)
		}
	}
}
