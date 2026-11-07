// Validador do bloco kids: (spec da frente B, seção 3.2). Junta as regras
// de text.go e numbers.go e devolve todos os problemas de uma espécie de
// uma vez, para quem revisa (ou o kidsgen, que manda os erros de volta ao
// modelo) ver tudo o que falta corrigir.

package kidscontent

import (
	"fmt"
	"slices"
	"strings"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// report junta os problemas de uma espécie, no formato
// "<id>: <campo>: <motivo>".
type report struct {
	id   string
	errs []error
}

func (r *report) add(field, reason string) {
	r.errs = append(r.errs, fmt.Errorf("%s: %s: %s", r.id, field, reason))
}

// Validate confere o bloco kids: de uma espécie e devolve todos os problemas
// (nil = válido). Sem bloco kids:, não há o que conferir: a regra do bioma
// principal sem Kids fica no loader (domain.Species.Validate), e o catálogo
// pode crescer com espécies que ainda não passaram pelo kidsgen.
func Validate(s domain.Species) []error {
	k := s.Kids
	if k == nil {
		return nil
	}
	r := &report{id: s.ID}
	switch {
	case s.PrimaryBiome == "":
		r.add("primary_biome", "obrigatório quando há o bloco kids")
	case !slices.Contains(s.Biomes, s.PrimaryBiome):
		r.add("primary_biome", fmt.Sprintf("%q não está entre os biomas da espécie", s.PrimaryBiome))
	}
	if strings.TrimSpace(k.GeneratedBy) == "" {
		r.add("kids.generated_by", "obrigatório")
	}
	if k.SourceHash != SourceHash(s) {
		r.add("kids.source_hash", "o texto adulto mudou desde a geração; rode o kidsgen de novo para esta espécie")
	}
	if n := len(k.Facts); n == 0 || n > len(s.Facts) {
		r.add("kids.facts", fmt.Sprintf("de 1 a %d fatos (um por fato adulto), veio %d", len(s.Facts), n))
	}
	seen := map[int]bool{}
	for i, f := range k.Facts {
		field := fmt.Sprintf("kids.facts[%d]", i)
		switch {
		case f.SourceFact < 0 || f.SourceFact >= len(s.Facts):
			r.add(field, fmt.Sprintf("source_fact %d não existe (há %d fatos adultos)", f.SourceFact, len(s.Facts)))
		case seen[f.SourceFact]:
			r.add(field, fmt.Sprintf("source_fact %d repetido", f.SourceFact))
		}
		seen[f.SourceFact] = true
	}

	adult := numberSet(adultText(s))
	r.text("kids.description", k.Description, 2, 4, adult)
	for i, f := range k.Facts {
		r.text(fmt.Sprintf("kids.facts[%d]", i), f.Text, 1, 1, adult)
	}
	r.text("kids.did_you_know", k.DidYouKnow, 1, 1, adult)
	return r.errs
}

// adultText junta a descrição e os fatos adultos: é nele que a regra dos
// números procura cada número do Kids.
func adultText(s domain.Species) string {
	parts := []string{s.Description}
	for _, f := range s.Facts {
		parts = append(parts, f.Text)
	}
	return strings.Join(parts, "\n")
}

// text confere um campo de texto do Kids: o número de frases, as palavras de
// cada frase, os números e as palavras proibidas.
func (r *report) text(field, text string, minSentences, maxSentences int, adult map[float64]bool) {
	if strings.TrimSpace(text) == "" {
		r.add(field, "vazio")
		return
	}
	sentences := Sentences(text)
	if n := len(sentences); n < minSentences || n > maxSentences {
		if minSentences == maxSentences {
			r.add(field, fmt.Sprintf("precisa de exatamente %d frase, tem %d", minSentences, n))
		} else {
			r.add(field, fmt.Sprintf("precisa de %d a %d frases, tem %d", minSentences, maxSentences, n))
		}
	}
	for _, s := range sentences {
		if n := Words(s); n > MaxWords {
			r.add(field, fmt.Sprintf("frase com %d palavras (máximo %d)", n, MaxWords))
		}
	}
	for _, v := range Numbers(text) {
		if !adult[numberKey(v)] {
			r.add(field, fmt.Sprintf("o número %s não aparece no texto adulto", formatNumber(v)))
		}
	}
	if w := ForbiddenWord(text); w != "" {
		r.add(field, fmt.Sprintf("palavra proibida %q", w))
	}
}
