// Package safehttp baixa arquivos de fontes externas de forma segura.
//
// SSRF (Server-Side Request Forgery, OWASP API7): se o worker baixasse
// QUALQUER URL que uma API externa mandasse, um atacante poderia fazê-lo
// acessar endereços internos (o banco, o serviço de metadados da nuvem em
// 169.254.169.254...). Por isso:
//  1. só HTTPS;
//  2. só hosts de uma lista permitida (allowlist);
//  3. o IP é checado NA HORA DE CONECTAR (depois do DNS), então nem um DNS
//     "malicioso" nem um redirecionamento escapam;
//  4. limite de tamanho e de Content-Type.
package safehttp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

var ErrBlocked = errors.New("download bloqueado")

type Options struct {
	AllowedHosts    []string
	AllowPrivateIPs bool // SÓ para testes locais
	Timeout         time.Duration
	MaxImageBytes   int64
	MaxAudioBytes   int64
	UserAgent       string
	Blocked         func(reason string) // chamada a cada bloqueio (métrica)
}

type Downloader struct {
	opts    Options
	allowed map[string]bool
	client  *http.Client
}

var _ ingest.Downloader = (*Downloader)(nil)

var allowedTypes = map[ingest.DownloadKind]map[string]bool{
	ingest.DownloadImage: {"image/jpeg": true, "image/png": true},
	ingest.DownloadAudio: {"audio/mpeg": true, "audio/mp3": true, "audio/wav": true, "audio/x-wav": true, "audio/wave": true},
}

func New(opts Options) *Downloader { return NewWithTransport(opts, nil) }

// NewWithTransport permite injetar uma config TLS (usado nos testes para
// confiar no certificado do servidor de teste).
func NewWithTransport(opts Options, tlsConfig *tls.Config) *Downloader {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxImageBytes == 0 {
		opts.MaxImageBytes = 15 << 20 // 15 MB
	}
	if opts.MaxAudioBytes == 0 {
		opts.MaxAudioBytes = 40 << 20 // 40 MB (WAV é grande)
	}
	d := &Downloader{opts: opts, allowed: map[string]bool{}}
	for _, h := range opts.AllowedHosts {
		d.allowed[strings.ToLower(h)] = true
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: d.checkIP}
	d.client = &http.Client{
		Timeout: opts.Timeout,
		Transport: &http.Transport{
			DialContext:     dialer.DialContext,
			TLSClientConfig: tlsConfig,
			Proxy:           nil, // sem proxy: a checagem de IP precisa ver o destino real
		},
		// Cada redirecionamento passa pelas mesmas regras de URL.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return d.block("redirecionamentos demais")
			}
			return d.checkURL(req.URL)
		},
	}
	return d
}

func (d *Downloader) block(reason string) error {
	if d.opts.Blocked != nil {
		d.opts.Blocked(reason)
	}
	return fmt.Errorf("%w: %s", ErrBlocked, reason)
}

func (d *Downloader) checkURL(u *url.URL) error {
	if u.Scheme != "https" {
		return d.block("esquema não é https")
	}
	if !d.allowed[strings.ToLower(u.Hostname())] {
		return d.block("host fora da allowlist: " + u.Hostname())
	}
	return nil
}

// checkIP roda DEPOIS da resolução de DNS, com o IP real da conexão.
func (d *Downloader) checkIP(_, address string, _ syscall.RawConn) error {
	if d.opts.AllowPrivateIPs {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return d.block("endereço inválido")
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return d.block("IP não permitido: " + host)
	}
	return nil
}

func (d *Downloader) Fetch(ctx context.Context, rawURL string, kind ingest.DownloadKind) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", d.block("URL inválida")
	}
	if err := d.checkURL(u); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	if d.opts.UserAgent != "" {
		req.Header.Set("User-Agent", d.opts.UserAgent)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrBlocked) {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("baixar: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("baixar: status %d", resp.StatusCode)
	}
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if !allowedTypes[kind][ct] {
		return nil, "", d.block("content-type não permitido: " + ct)
	}
	limit := d.opts.MaxImageBytes
	if kind == ingest.DownloadAudio {
		limit = d.opts.MaxAudioBytes
	}
	// Lemos no máximo limit+1 bytes: se vier mais, o arquivo é grande demais.
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("ler corpo: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, "", d.block("arquivo maior que o limite")
	}
	return data, ct, nil
}
