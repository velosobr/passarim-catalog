// Package media converte fotos e áudios usando o ffmpeg (programa externo).
// Chamamos o binário em vez de uma biblioteca Go porque nenhuma biblioteca
// em Go puro codifica WebP com perdas E AAC. O ffmpeg faz os dois.
package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // registra o decodificador JPEG para image.DecodeConfig
	_ "image/png"  // idem para PNG
	"os/exec"
	"strconv"
	"time"

	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

const MaxPixels = 40_000_000 // 40 megapixels

var (
	ErrTooLarge    = errors.New("imagem grande demais")
	ErrUnsupported = errors.New("formato não suportado")
)

type FFmpeg struct {
	binary string
}

var (
	_ ingest.ImageProcessor = (*FFmpeg)(nil)
	_ ingest.AudioProcessor = (*FFmpeg)(nil)
)

func NewFFmpeg(binary string) *FFmpeg { return &FFmpeg{binary: binary} }

// Process gera as 3 variantes WebP. Antes de tudo lê SÓ o cabeçalho da
// imagem: uma "bomba de descompressão" (arquivo pequeno que vira bilhões
// de pixels) é recusada sem gastar memória (OWASP API10).
func (f *FFmpeg) Process(ctx context.Context, original []byte) (ingest.PhotoVariants, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		return ingest.PhotoVariants{}, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return ingest.PhotoVariants{}, fmt.Errorf("%w: %dx%d", ErrTooLarge, cfg.Width, cfg.Height)
	}
	var v ingest.PhotoVariants
	for _, spec := range []struct {
		width int
		dst   *[]byte
	}{{320, &v.Thumb}, {800, &v.Medium}, {1600, &v.Large}} {
		out, err := f.webp(ctx, original, spec.width)
		if err != nil {
			return ingest.PhotoVariants{}, err
		}
		*spec.dst = out
	}
	// Dimensões da large: largura limitada a 1600 sem ampliar; altura proporcional e par.
	v.Width = min(cfg.Width, 1600)
	v.Height = cfg.Height * v.Width / cfg.Width
	v.Height -= v.Height % 2
	return v, nil
}

func (f *FFmpeg) webp(ctx context.Context, in []byte, width int) ([]byte, error) {
	// scale='min(W,iw)':-2 = largura no máximo W (nunca amplia), altura
	// proporcional e PAR (-2), exigência de vários codificadores.
	filter := "scale='min(" + strconv.Itoa(width) + ",iw)':-2"
	return f.run(ctx, in, "-i", "pipe:0", "-vf", filter, "-c:v", "libwebp", "-quality", "80", "-f", "webp", "pipe:1")
}

// ToAAC converte para AAC mono 96 kbps, cortando em maxDuration.
// Usamos o formato ADTS (AAC "cru"), que pode ser escrito num pipe.
func (f *FFmpeg) ToAAC(ctx context.Context, original []byte, maxDuration time.Duration) ([]byte, error) {
	secs := strconv.FormatFloat(maxDuration.Seconds(), 'f', 3, 64)
	return f.run(ctx, original, "-i", "pipe:0", "-t", secs, "-ac", "1", "-c:a", "aac", "-b:a", "96k", "-f", "adts", "pipe:1")
}

func (f *FFmpeg) run(ctx context.Context, in []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	full := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin"}, args...)
	// gosec G204: os argumentos são fixos no código; só os BYTES vêm de fora (stdin).
	cmd := exec.CommandContext(ctx, f.binary, full...) //nolint:gosec // argumentos constantes
	cmd.Stdin = bytes.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
