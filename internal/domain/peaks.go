// Picos do canto: 64 números de 0 a 1 que o app usa para desenhar a forma
// de onda real da gravação (frente B). A conta é uma regra pura: recebe as
// amostras do som e devolve os picos, sem ffmpeg nem banco, e por isso é
// testada com amostras feitas à mão.

package domain

import "math"

// PeakCount é quantos picos cada canto tem: uma barra por pico no player.
const PeakCount = 64

// Peaks divide as amostras em 64 janelas iguais (a última leva o resto) e
// guarda o maior valor absoluto de cada uma. Depois normaliza: divide tudo
// pelo maior dos 64, para a barra mais alta valer 1, e arredonda a 3 casas.
//
// Uma amostra é a "altura" do som num instante: o PCM de 16 bits vai de
// -32768 a 32767. O absoluto é calculado em int32 porque o de -32768 não
// cabe em int16.
//
// Silêncio total dá 64 zeros (sem divisão por zero). Com menos de 64
// amostras não há como formar as janelas: devolve nil, e o app mostra uma
// forma genérica.
func Peaks(samples []int16) []float32 {
	if len(samples) < PeakCount {
		return nil
	}
	size := len(samples) / PeakCount
	raw := make([]int32, PeakCount)
	var highest int32
	for i := range PeakCount {
		start, end := i*size, (i+1)*size
		if i == PeakCount-1 {
			end = len(samples)
		}
		for _, s := range samples[start:end] {
			v := int32(s)
			if v < 0 {
				v = -v
			}
			raw[i] = max(raw[i], v)
		}
		highest = max(highest, raw[i])
	}
	out := make([]float32, PeakCount)
	if highest == 0 {
		return out
	}
	for i, v := range raw {
		out[i] = float32(math.Round(float64(v)/float64(highest)*1000) / 1000)
	}
	return out
}
