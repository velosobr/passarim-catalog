package safehttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/adapter/safehttp"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

func host(t *testing.T, srv *httptest.Server) string {
	u, _ := url.Parse(srv.URL)
	return u.Hostname()
}

func imageServer() *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-jpeg-bytes"))
	}))
}

// newTestDownloader confia no certificado do servidor de teste.
func newTestDownloader(srv *httptest.Server, opts safehttp.Options) *safehttp.Downloader {
	opts.Timeout = 2 * time.Second
	if opts.MaxImageBytes == 0 {
		opts.MaxImageBytes = 1 << 20
	}
	return safehttp.NewWithTransport(opts, srv.Client().Transport.(*http.Transport).TLSClientConfig)
}

func TestDownloader_AllowsListedHostWhenPrivateAllowed(t *testing.T) {
	srv := imageServer()
	defer srv.Close()
	d := newTestDownloader(srv, safehttp.Options{AllowedHosts: []string{host(t, srv)}, AllowPrivateIPs: true})
	data, ct, err := d.Fetch(context.Background(), srv.URL+"/a.jpg", ingest.DownloadImage)
	if err != nil || string(data) != "fake-jpeg-bytes" || ct != "image/jpeg" {
		t.Fatalf("got %q %q %v", data, ct, err)
	}
}

func TestDownloader_BlocksHostNotInAllowlist(t *testing.T) {
	srv := imageServer()
	defer srv.Close()
	d := newTestDownloader(srv, safehttp.Options{AllowedHosts: []string{"inaturalist-open-data.s3.amazonaws.com"}, AllowPrivateIPs: true})
	_, _, err := d.Fetch(context.Background(), srv.URL+"/a.jpg", ingest.DownloadImage)
	if !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("host fora da allowlist deveria ser bloqueado, veio %v", err)
	}
}

func TestDownloader_BlocksPlainHTTP(t *testing.T) {
	d := safehttp.New(safehttp.Options{AllowedHosts: []string{"example.org"}})
	_, _, err := d.Fetch(context.Background(), "http://example.org/a.jpg", ingest.DownloadImage)
	if !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("http:// deveria ser bloqueado, veio %v", err)
	}
}

func TestDownloader_BlocksPrivateIP(t *testing.T) {
	srv := imageServer() // escuta em 127.0.0.1
	defer srv.Close()
	// Host na allowlist, mas o IP é loopback: deve bloquear mesmo assim.
	d := newTestDownloader(srv, safehttp.Options{AllowedHosts: []string{host(t, srv)}, AllowPrivateIPs: false})
	_, _, err := d.Fetch(context.Background(), srv.URL+"/a.jpg", ingest.DownloadImage)
	if !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("IP privado deveria ser bloqueado, veio %v", err)
	}
}

// Review Focus #1
func TestDownloader_BlocksPrivateIPAfterRedirect(t *testing.T) {
	var target *httptest.Server
	target = imageServer()
	defer target.Close()
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer redirector.Close()
	d := newTestDownloader(redirector, safehttp.Options{AllowedHosts: []string{host(t, redirector), "169.254.169.254"}, AllowPrivateIPs: false})
	_, _, err := d.Fetch(context.Background(), redirector.URL+"/a.jpg", ingest.DownloadImage)
	if !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("redirecionamento para IP de metadados deveria ser bloqueado, veio %v", err)
	}
}

func TestDownloader_RejectsOversizedBody(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
	}))
	defer srv.Close()
	d := newTestDownloader(srv, safehttp.Options{AllowedHosts: []string{host(t, srv)}, AllowPrivateIPs: true, MaxImageBytes: 1024})
	if _, _, err := d.Fetch(context.Background(), srv.URL+"/a.jpg", ingest.DownloadImage); !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("corpo maior que o limite deveria ser recusado, veio %v", err)
	}
}

func TestDownloader_RejectsUnexpectedContentType(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>"))
	}))
	defer srv.Close()
	d := newTestDownloader(srv, safehttp.Options{AllowedHosts: []string{host(t, srv)}, AllowPrivateIPs: true})
	if _, _, err := d.Fetch(context.Background(), srv.URL+"/a.jpg", ingest.DownloadImage); !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("text/html não é imagem, veio %v", err)
	}
}

func TestDownloader_ReportsBlockedReason(t *testing.T) {
	var reasons []string
	d := safehttp.New(safehttp.Options{AllowedHosts: []string{"example.org"}, Blocked: func(r string) { reasons = append(reasons, r) }})
	_, _, _ = d.Fetch(context.Background(), "http://example.org/a.jpg", ingest.DownloadImage)
	if len(reasons) != 1 {
		t.Fatalf("Blocked deveria ser chamado uma vez (vira métrica de segurança), veio %v", reasons)
	}
}
