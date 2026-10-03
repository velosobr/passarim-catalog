package xenocanto_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"github.com/velosobr/passarim-catalog/internal/adapter/sources/xenocanto"
)

const testKey = "segredo-de-teste"

func limiter() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

func TestFindRecordings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/3/recordings" || !strings.Contains(q.Get("query"), `sp:"Turdus rufiventris"`) ||
			!strings.Contains(q.Get("query"), "cnt:brazil") || q.Get("key") != testKey {
			t.Errorf("requisição errada: %s", r.URL)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent ausente")
		}
		b, _ := os.ReadFile("testdata/recordings.json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	recs, err := xenocanto.New(srv.URL, testKey, "ua", limiter()).FindRecordings(context.Background(), "Turdus rufiventris")
	if err != nil || len(recs) == 0 {
		t.Fatalf("got %v %v", recs, err)
	}
	r := recs[0]
	if r.SourceID != "1172942" {
		t.Errorf("SourceID deveria ser o id da gravação, veio %q", r.SourceID)
	}
	if r.DurationMs != 86000 {
		t.Errorf("1:26 deveria virar 86000 ms, veio %d", r.DurationMs)
	}
	if !strings.HasPrefix(r.License, "https://") || r.Author == "" || r.URL == "" || r.PageURL == "" || r.Quality == "" {
		t.Errorf("campos incompletos: %+v", r)
	}
}

func TestFindRecordings_ProtocolRelativeLicense(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"recordings":[{"file":"https://xeno-canto.org/1/download","lic":"//creativecommons.org/licenses/by/4.0/","rec":"A","url":"https://xeno-canto.org/1","length":"0:07","q":"A","type":"song"}]}`))
	}))
	defer srv.Close()
	recs, err := xenocanto.New(srv.URL, testKey, "ua", limiter()).FindRecordings(context.Background(), "A b")
	if err != nil || len(recs) != 1 || recs[0].License != "https://creativecommons.org/licenses/by/4.0/" {
		t.Fatalf("got %+v %v", recs, err)
	}
}

func TestFindRecordings_ErrorDoesNotLeakKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer srv.Close()
	_, err := xenocanto.New(srv.URL, testKey, "ua", limiter()).FindRecordings(context.Background(), "A b")
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Fatalf("erro deveria existir e não conter a chave: %v", err)
	}
}

func TestFindRecordings_ConnectionErrorDoesNotLeakKey(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // conexão recusada: o erro do net/http citaria a URL (com a chave)
	_, err := xenocanto.New(url, testKey, "ua", limiter()).FindRecordings(context.Background(), "A b")
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Fatalf("erro de conexão não pode conter a chave: %v", err)
	}
}
