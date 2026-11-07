// Testes do cálculo dos 64 picos do canto, com amostras feitas à mão.

package domain_test

import (
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// Uma rampa de 0 a 6399 em 6400 amostras: cada janela tem 100 amostras, e o
// pico da janela i é 100*i+99. Normalizado pelo maior (6399), a última vale 1.
func TestPeaks_Ramp(t *testing.T) {
	samples := make([]int16, 6400)
	for i := range samples {
		samples[i] = int16(i)
	}
	p := domain.Peaks(samples)
	if len(p) != domain.PeakCount || p[63] != 1 || p[0] != 0.015 {
		t.Fatalf("len=%d primeiro=%v último=%v", len(p), p[0], p[63])
	}
	for i := 1; i < len(p); i++ {
		if p[i] < p[i-1] {
			t.Fatalf("a rampa deveria crescer: p[%d]=%v < p[%d]=%v", i, p[i], i-1, p[i-1])
		}
	}
}

// O pico é o maior valor ABSOLUTO: uma amostra negativa grande conta.
func TestPeaks_NegativeCounts(t *testing.T) {
	samples := make([]int16, 128)
	samples[0] = -32768 // o menor int16; o absoluto dele não cabe em int16
	samples[127] = 16384
	p := domain.Peaks(samples)
	if p[0] != 1 || p[63] != 0.5 {
		t.Fatalf("primeiro=%v último=%v", p[0], p[63])
	}
}

// Silêncio total dá 64 zeros, sem divisão por zero.
func TestPeaks_Silence(t *testing.T) {
	p := domain.Peaks(make([]int16, 8000))
	if len(p) != domain.PeakCount {
		t.Fatalf("len=%d", len(p))
	}
	for _, v := range p {
		if v != 0 {
			t.Fatalf("silêncio deveria dar zeros, veio %v", p)
		}
	}
}

// Menos de 64 amostras: sem picos (lista vazia), sem erro.
func TestPeaks_TooShort(t *testing.T) {
	if p := domain.Peaks(make([]int16, 63)); len(p) != 0 {
		t.Fatalf("esperava vazio, veio %d", len(p))
	}
}

// 130 amostras: 63 janelas de 2 e a última com o resto (4). O pico que só
// está no fim cai na última janela.
func TestPeaks_LastWindowTakesRemainder(t *testing.T) {
	samples := make([]int16, 130)
	samples[129] = 1000
	samples[0] = 500
	p := domain.Peaks(samples)
	if p[63] != 1 || p[0] != 0.5 {
		t.Fatalf("primeiro=%v último=%v", p[0], p[63])
	}
}
