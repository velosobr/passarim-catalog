// Testes do conversor com o ffmpeg de verdade. Sem o ffmpeg instalado, os
// testes que precisam dele são pulados, a não ser com REQUIRE_FFMPEG=1 (como
// na CI), que os faz falhar.

package media_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/adapter/media"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// requireFFmpeg pula o teste quando o ffmpeg não está no PATH, ou o reprova
// se REQUIRE_FFMPEG=1.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		// No CI (REQUIRE_FFMPEG=1) pular seria esconder testes: falha de verdade.
		if os.Getenv("REQUIRE_FFMPEG") == "1" {
			t.Fatal("ffmpeg não instalado, mas REQUIRE_FFMPEG=1")
		}
		t.Skip("ffmpeg não instalado")
	}
}

// pngOf gera uma imagem PNG lisa do tamanho pedido.
func pngOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// Uma imagem vira três WebP, com as dimensões da grande calculadas.
func TestImageProcessor_CreatesThreeWebPVariants(t *testing.T) {
	requireFFmpeg(t)
	v, err := processAll(context.Background(), media.NewFFmpeg("ffmpeg"), pngOf(2000, 1000))
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

// noisyPNGOf gera uma PNG com ruído, que comprime mal e produz arquivos
// maiores (bom para pegar arquivos cortados).
func noisyPNGOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	_, _ = rand.NewChaCha8([32]byte{1}).Read(img.Pix)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// Os WebP gerados são arquivos completos e válidos.
func TestImageProcessor_WritesCompleteWebPFiles(t *testing.T) {
	requireFFmpeg(t)
	v, err := processAll(context.Background(), media.NewFFmpeg("ffmpeg"), noisyPNGOf(2000, 1000))
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"thumb": v.Thumb, "medium": v.Medium, "large": v.Large} {
		if got, want := int(binary.LittleEndian.Uint32(b[4:8])), len(b)-8; got != want {
			t.Errorf("%s: tamanho do RIFF %d, esperado %d", name, got, want)
		}
		end := 12
		for end+8 <= len(b) {
			size := int(binary.LittleEndian.Uint32(b[end+4 : end+8]))
			end += 8 + size + size%2
		}
		if end != len(b) {
			t.Errorf("%s: os chunks terminam em %d, o arquivo em %d", name, end, len(b))
		}
	}
}

// Uma imagem menor que 1600 px não é ampliada.
func TestImageProcessor_DoesNotUpscale(t *testing.T) {
	requireFFmpeg(t)
	v, err := processAll(context.Background(), media.NewFFmpeg("ffmpeg"), pngOf(500, 250))
	if err != nil || v.Width != 500 || v.Height != 250 {
		t.Fatalf("imagem pequena não pode ser ampliada: %dx%d %v", v.Width, v.Height, err)
	}
}

// Rejeitado ANTES de chamar o ffmpeg (não precisa dele).
func TestImageProcessor_RejectsHugeDimensions(t *testing.T) {
	// Cabeçalho PNG declarando 20000x20000 = 400 MP (o arquivo tem poucos bytes).
	huge := pngHeader(20000, 20000)
	if _, err := processAll(context.Background(), media.NewFFmpeg("ffmpeg-que-nao-existe"), huge); !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("esperava ErrTooLarge, veio %v", err)
	}
}

// Bytes que não são imagem são recusados.
func TestImageProcessor_RejectsNonImage(t *testing.T) {
	if _, err := processAll(context.Background(), media.NewFFmpeg("ffmpeg"), []byte("not an image")); !errors.Is(err, media.ErrUnsupported) {
		t.Fatalf("esperava ErrUnsupported, veio %v", err)
	}
}

// O áudio vira AAC e é cortado na duração máxima.
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
	ihdr := append([]byte("IHDR"), make([]byte, 13)...)
	binary.BigEndian.PutUint32(ihdr[4:], w)
	binary.BigEndian.PutUint32(ihdr[8:], h)
	copy(ihdr[12:], []byte{8, 2, 0, 0, 0}) // 8 bits, RGB, sem entrelaçamento
	buf.Write([]byte{0, 0, 0, 13})
	buf.Write(ihdr)
	buf.Write(binary.BigEndian.AppendUint32(nil, crc32.ChecksumIEEE(ihdr)))
	return buf.Bytes()
}

// processAll gera as três variantes pelos dois passos do worker (a medium
// para o curador; thumb e large só para as escolhidas), para os testes
// antigos continuarem conferindo as três juntas.
func processAll(ctx context.Context, f *media.FFmpeg, in []byte) (ingest.PhotoVariants, error) {
	medium, err := f.Medium(ctx, in)
	if err != nil {
		return ingest.PhotoVariants{}, err
	}
	v, err := f.ThumbAndLarge(ctx, in)
	if err != nil {
		return ingest.PhotoVariants{}, err
	}
	v.Medium = medium
	return v, nil
}

// ThumbAndLarge não gera a medium (ela já foi gerada para o curador).
func TestImageProcessor_ThumbAndLargeLeavesMediumOut(t *testing.T) {
	requireFFmpeg(t)
	v, err := media.NewFFmpeg("ffmpeg").ThumbAndLarge(context.Background(), pngOf(2000, 1000))
	if err != nil || len(v.Thumb) == 0 || len(v.Large) == 0 || v.Medium != nil || v.Width != 1600 || v.Height != 800 {
		t.Fatalf("got thumb=%d large=%d medium=%v %dx%d err=%v", len(v.Thumb), len(v.Large), v.Medium != nil, v.Width, v.Height, err)
	}
}

// Medium também recusa a bomba de descompressão, sem chamar o ffmpeg.
func TestImageProcessor_MediumRejectsHugeDimensions(t *testing.T) {
	if _, err := media.NewFFmpeg("ffmpeg-que-nao-existe").Medium(context.Background(), pngHeader(100000, 100000)); !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("esperava ErrTooLarge, veio %v", err)
	}
}
