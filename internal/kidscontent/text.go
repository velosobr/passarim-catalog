// Regras de texto do conteúdo Kids (frente B): como cortar frases, contar
// palavras, achar palavras proibidas e calcular o "carimbo" do texto adulto.
// Ficam num pacote só, usado pelo teste do conteúdo (o "lint") e pelo
// cmd/kidsgen, para os dois nunca discordarem sobre o que é um texto válido.

package kidscontent

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// MaxWords é o máximo de palavras numa frase do Kids.
const MaxWords = 15

// sentenceEnd acha o fim de uma frase: um ou mais ".", "!" ou "?" seguidos
// de espaço em branco ou do fim do texto. Assim o ponto de "2.000" não corta
// a frase, porque vem seguido de um algarismo. regexp é o pacote de
// expressões regulares do Go: \s é qualquer espaço em branco e $, o fim.
var sentenceEnd = regexp.MustCompile(`[.!?]+(\s+|$)`)

// Sentences divide o texto em frases, sem espaços nas pontas. Um trecho no
// fim sem pontuação também conta como frase: o validador conta frases e
// palavras, não exige o ponto final.
func Sentences(text string) []string {
	var out []string
	start := 0
	for _, loc := range sentenceEnd.FindAllStringIndex(text, -1) {
		if s := strings.TrimSpace(text[start:loc[1]]); s != "" {
			out = append(out, s)
		}
		start = loc[1]
	}
	if s := strings.TrimSpace(text[start:]); s != "" {
		out = append(out, s)
	}
	return out
}

// Words conta as palavras de uma frase: os trechos entre espaços que têm
// pelo menos uma letra ou um algarismo. "tico-tico" é uma palavra só, e um
// travessão solto ("—") não conta.
func Words(sentence string) int {
	n := 0
	for _, f := range strings.Fields(sentence) {
		if strings.IndexFunc(f, isLetterOrDigit) >= 0 {
			n++
		}
	}
	return n
}

func isLetterOrDigit(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// notLetter diz se o caractere separa palavras (tudo o que não é letra).
func notLetter(r rune) bool { return !unicode.IsLetter(r) }

// forbidden são começos de palavra que o Kids nunca usa, já sem acento e em
// minúsculas. A lista é curta de propósito: o resto da suavização fica com o
// prompt e com a revisão humana (spec da frente B, seção 3.4).
var forbidden = []string{"sangue", "ensanguentad", "doenca", "doente", "cadaver", "carcaca", "abate", "matanca"}

// ForbiddenWord devolve a primeira palavra do texto que começa com um termo
// proibido ("" se não houver). A comparação ignora acentos e maiúsculas:
// "Doença" e "doenca" são a mesma palavra.
func ForbiddenWord(text string) string {
	for _, w := range strings.FieldsFunc(text, notLetter) {
		n := domain.NormalizeForSearch(w)
		for _, f := range forbidden {
			if strings.HasPrefix(n, f) {
				return w
			}
		}
	}
	return ""
}

// Collapse troca qualquer sequência de espaços em branco (inclusive quebras
// de linha) por um espaço só e apara as pontas.
func Collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// SourceHash é o "carimbo" do texto adulto de onde o Kids saiu: o SHA-256,
// em hexadecimal com o prefixo "sha256:", de um texto canônico com três
// partes separadas por "\n" (spec, seção 3.1):
//  1. a descrição adulta com os espaços colapsados;
//  2. os fatos adultos, na ordem, colapsados e separados por "\n";
//  3. os biomas em ordem alfabética, separados por vírgula.
//
// SHA-256 é uma função de hash: o mesmo texto dá sempre o mesmo carimbo, e
// mudar uma letra muda o carimbo inteiro. Mudar a descrição, um fato ou os
// biomas pede uma nova geração; mudar só as fontes, o tamanho ou as UFs, não.
func SourceHash(s domain.Species) string {
	facts := make([]string, len(s.Facts))
	for i, f := range s.Facts {
		facts[i] = Collapse(f.Text)
	}
	biomes := make([]string, len(s.Biomes))
	for i, b := range s.Biomes {
		biomes[i] = string(b)
	}
	slices.Sort(biomes)
	canonical := Collapse(s.Description) + "\n" + strings.Join(facts, "\n") + "\n" + strings.Join(biomes, ",")
	sum := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(sum[:])
}
