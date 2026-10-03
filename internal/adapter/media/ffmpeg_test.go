package media_test

import (
	"bytes"
	"context"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/adapter/media"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não instalado")
	}
}

func pngOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestImageProcessor_CreatesThreeWebPVariants(t *testing.T) {
	requireFFmpeg(t)
	v, err := media.NewFFmpeg("ffmpeg").Process(context.Background(), pngOf(2000, 1000))
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"thumb": v.Thumb, "medium": v.Medium, "large": v.Large} {
		// Arquivos WebP começam com "RIFF....WEBP".
		if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
			t.Errorf("%s não é WebP", name)
		}
	}
	if v.Width != 1600 || v.Height != 800 {
		t.Fatalf("large deveria ter 1600x800, veio %dx%d", v.Width, v.Height)
	}
}

func TestImageProcessor_DoesNotUpscale(t *testing.T) {
	requireFFmpeg(t)
	v, err := media.NewFFmpeg("ffmpeg").Process(context.Background(), pngOf(500, 250))
	if err != nil || v.Width != 500 || v.Height != 250 {
		t.Fatalf("imagem pequena não pode ser ampliada: %dx%d %v", v.Width, v.Height, err)
	}
}

// Review Focus #3: rejeitado ANTES de chamar o ffmpeg (não precisa dele).
func TestImageProcessor_RejectsHugeDimensions(t *testing.T) {
	// Cabeçalho PNG declarando 20000x20000 = 400 MP (o arquivo tem poucos bytes).
	huge := pngHeader(20000, 20000)
	if _, err := media.NewFFmpeg("ffmpeg-que-nao-existe").Process(context.Background(), huge); !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("esperava ErrTooLarge, veio %v", err)
	}
}

func TestImageProcessor_RejectsNonImage(t *testing.T) {
	if _, err := media.NewFFmpeg("ffmpeg").Process(context.Background(), []byte("not an image")); !errors.Is(err, media.ErrUnsupported) {
		t.Fatalf("esperava ErrUnsupported, veio %v", err)
	}
}

func TestAudioProcessor_TrimsToAAC(t *testing.T) {
	requireFFmpeg(t)
	// Gera 5 s de tom em WAV com o próprio ffmpeg.
	wav, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=1000:duration=5",
		"-f", "wav", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	out, err := media.NewFFmpeg("ffmpeg").ToAAC(context.Background(), wav, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// ADTS (AAC "cru") começa com o syncword 0xFFF.
	if len(out) < 2 || out[0] != 0xFF || out[1]&0xF0 != 0xF0 {
		t.Fatalf("saída não é AAC/ADTS: % x", out[:min(4, len(out))])
	}
	if len(out) > len(wav) {
		t.Fatalf("AAC cortado em 2 s deveria ser bem menor que o WAV de 5 s")
	}
}

// pngHeader devolve o início de um PNG válido (assinatura + chunk IHDR)
// declarando w x h. image.DecodeConfig lê só isso.
func pngHeader(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	ihdr := []byte{'I', 'H', 'D', 'R',
		byte(w >> 24), byte(w >> 16), byte(w >> 8), byte(w),
		byte(h >> 24), byte(h >> 16), byte(h >> 8), byte(h),
		8, 2, 0, 0, 0}
	buf.Write([]byte{0, 0, 0, 13})
	buf.Write(ihdr)
	crc := crc32.ChecksumIEEE(ihdr)
	buf.Write([]byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)})
	return buf.Bytes()
}
