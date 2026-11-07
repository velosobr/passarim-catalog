// Regra dos números do Kids (spec da frente B, seção 3.3): todo número do
// texto Kids tem que aparecer no texto adulto, para a IA não inventar
// quantidades. Este arquivo acha os números de um texto, escritos com
// algarismos ("2.000", "1,5") ou por extenso ("vinte e cinco", "dois mil",
// "meio"), e devolve o VALOR de cada um: assim "2 minutos" casa com "dois
// minutos".

package kidscontent

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// digitsPattern acha um número com algarismos, com "." e "," entre os grupos
// ("2.000", "1,5", "2002"). Um ponto final logo depois não entra, porque o
// padrão exige um algarismo depois de cada separador.
var digitsPattern = regexp.MustCompile(`\d+(?:[.,]\d+)*`)

// wordValues são as palavras que valem um número sozinhas, já sem acento
// (o texto passa por domain.NormalizeForSearch antes). "um" e "uma" não
// estão aqui: só contam dentro de um número maior ("vinte e um", "um mil"),
// porque quase sempre são artigos ("uma ave").
var wordValues = map[string]float64{
	"zero": 0, "dois": 2, "duas": 2, "tres": 3, "quatro": 4, "cinco": 5, "seis": 6, "sete": 7, "oito": 8, "nove": 9,
	"dez": 10, "onze": 11, "doze": 12, "treze": 13, "catorze": 14, "quatorze": 14, "quinze": 15, "dezesseis": 16,
	"dezessete": 17, "dezoito": 18, "dezenove": 19, "vinte": 20, "trinta": 30, "quarenta": 40, "cinquenta": 50,
	"sessenta": 60, "setenta": 70, "oitenta": 80, "noventa": 90, "cem": 100, "cento": 100,
	"duzentos": 200, "duzentas": 200, "trezentos": 300, "trezentas": 300, "quatrocentos": 400, "quatrocentas": 400,
	"quinhentos": 500, "quinhentas": 500, "seiscentos": 600, "seiscentas": 600, "setecentos": 700, "setecentas": 700,
	"oitocentos": 800, "oitocentas": 800, "novecentos": 900, "novecentas": 900,
}

// scales multiplicam a parte que vem antes: "dois mil" = 2 × 1000.
var scales = map[string]float64{"mil": 1e3, "milhao": 1e6, "milhoes": 1e6}

// Numbers devolve os valores dos números do texto: primeiro os escritos com
// algarismos, depois os por extenso, cada grupo na ordem do texto.
func Numbers(text string) []float64 {
	var out []float64
	for _, m := range digitsPattern.FindAllString(text, -1) {
		if v, ok := parseDigits(m); ok {
			out = append(out, v)
		}
	}
	return append(out, wordNumbers(text)...)
}

// parseDigits lê um número no padrão brasileiro: "," separa os decimais
// ("1,5" = 1.5) e "." separa os milhares ("2.000" = 2000). Um "." que não
// separa grupos de 3 algarismos é lido como decimal ("1.5"), como num texto
// em inglês.
func parseDigits(s string) (float64, bool) {
	if strings.Contains(s, ",") {
		s = strings.Replace(strings.ReplaceAll(s, ".", ""), ",", ".", 1)
	} else if parts := strings.Split(s, "."); len(parts) > 1 && thousandGroups(parts[1:]) {
		s = strings.Join(parts, "")
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

// thousandGroups diz se todos os grupos depois do primeiro têm 3 algarismos.
func thousandGroups(groups []string) bool {
	for _, g := range groups {
		if len(g) != 3 {
			return false
		}
	}
	return true
}

// isNumberWord diz se a palavra pode fazer parte de um número por extenso.
func isNumberWord(w string) bool {
	_, value := wordValues[w]
	_, scale := scales[w]
	return value || scale || w == "um" || w == "uma" || w == "meio" || w == "meia"
}

// wordNumbers lê os números por extenso. Vai palavra por palavra somando:
// current é a parte desde a última escala ("quinhentos") e total, o que já
// foi multiplicado por uma escala ("dois mil"). O "e" só liga as partes
// quando vem entre duas palavras de número ("vinte e cinco"); qualquer outra
// palavra fecha o número.
func wordNumbers(text string) []float64 {
	words := strings.FieldsFunc(domain.NormalizeForSearch(text), notLetter)
	var out []float64
	var total, current float64
	in := false // dentro de um número por extenso
	flush := func() {
		if in {
			out = append(out, total+current)
		}
		total, current, in = 0, 0, false
	}
	for i, w := range words {
		next, afterAnd := "", in && i > 0 && words[i-1] == "e"
		if i+1 < len(words) {
			next = words[i+1]
		}
		_, isScale := scales[next]
		switch value, isValue := wordValues[w]; {
		case w == "e" && in && isNumberWord(next):
			// O "e" de "vinte e cinco": nada a somar.
		case w == "um" || w == "uma":
			switch {
			case afterAnd: // "vinte e um"
				current++
			case isScale: // "um milhão"
				flush()
				in, current = true, 1
			default: // artigo: "uma ave"
				flush()
			}
		case w == "meio" || w == "meia":
			if afterAnd { // "dois e meio"
				current += 0.5
			} else {
				flush()
				out = append(out, 0.5)
			}
		case isValue:
			in = true
			current += value
		case scales[w] > 0:
			in = true
			total += math.Max(current, 1) * scales[w]
			current = 0
		default:
			flush()
		}
	}
	flush()
	return out
}

// numberKey arredonda o valor a 3 casas, para comparar números como chaves
// de um mapa sem tropeçar no arredondamento dos float (0,5 é sempre 0,5).
func numberKey(v float64) float64 { return math.Round(v*1000) / 1000 }

// numberSet devolve os valores dos números do texto, como conjunto.
func numberSet(text string) map[float64]bool {
	out := map[float64]bool{}
	for _, v := range Numbers(text) {
		out[numberKey(v)] = true
	}
	return out
}

// formatNumber escreve o valor como no erro ("2000", "1.5").
func formatNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
