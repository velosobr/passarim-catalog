// Testes do validador do bloco kids:, um caso por regra da spec (seção 3.2).

package kidscontent_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/kidscontent"
)

// adult devolve a espécie só com o texto adulto (sem Kids). Os espaços
// sobrando na descrição são de propósito: o carimbo os colapsa.
func adult() domain.Species {
	return domain.Species{
		ID: "turdus-rufiventris", CommonNamePt: "Sabiá-laranjeira",
		Description: "O sabiá  canta por dois minutos.\n Ele vive em quintais.",
		Facts: []domain.Fact{{Text: "Foi declarado ave nacional em 2002.", Source: "https://a"},
			{Text: "Imita outras aves.", Source: "https://b"}},
		Biomes: []domain.Biome{domain.BiomeMataAtlantica, domain.BiomeCerrado},
		States: []string{"SP"},
	}
}

// valid devolve a espécie com um bloco Kids que passa em todas as regras.
func valid() domain.Species {
	s := adult()
	s.PrimaryBiome = domain.BiomeMataAtlantica
	s.Kids = &domain.KidsContent{
		Description: "O sabiá tem a barriga laranja. Ele canta por 2 minutos.",
		Facts: []domain.KidsFact{{Text: "Em 2002 ele virou a ave do Brasil.", SourceFact: 0},
			{Text: "Ele imita outras aves.", SourceFact: 1}},
		DidYouKnow:  "O sabiá acorda bem cedinho.",
		GeneratedBy: "claude-opus-5-5",
		SourceHash:  kidscontent.SourceHash(adult()),
	}
	return s
}

func TestValidate_ValidAndAbsent(t *testing.T) {
	if errs := kidscontent.Validate(valid()); errs != nil {
		t.Fatalf("válida recusada: %v", errs)
	}
	if errs := kidscontent.Validate(adult()); errs != nil {
		t.Fatalf("sem bloco kids: não há o que validar, veio %v", errs)
	}
}

// Cada caso quebra uma regra e confere o trecho do erro.
func TestValidate_Rules(t *testing.T) {
	cases := map[string]struct {
		change func(*domain.Species)
		want   string
	}{
		"generated_by faltando": {func(s *domain.Species) { s.Kids.GeneratedBy = "" }, "kids.generated_by: obrigatório"},
		"bioma fora da lista":   {func(s *domain.Species) { s.PrimaryBiome = domain.BiomePampa }, `primary_biome: "pampa" não está`},
		"sem bioma principal":   {func(s *domain.Species) { s.PrimaryBiome = "" }, "primary_biome: obrigatório"},
		"hash velho":            {func(s *domain.Species) { s.Description = "Outro texto adulto. Bem diferente." }, "kids.source_hash:"},
		"source_fact inválido":  {func(s *domain.Species) { s.Kids.Facts[0].SourceFact = 5 }, "kids.facts[0]: source_fact 5 não existe"},
		"source_fact repetido":  {func(s *domain.Species) { s.Kids.Facts[1].SourceFact = 0 }, "kids.facts[1]: source_fact 0 repetido"},
		"fatos demais": {func(s *domain.Species) {
			s.Kids.Facts = append(s.Kids.Facts, domain.KidsFact{Text: "Mais um.", SourceFact: 1})
		}, "kids.facts: de 1 a 2 fatos"},
		"frase com 16 palavras":  {func(s *domain.Species) { s.Kids.DidYouKnow = strings.Repeat("pio ", 15) + "pio." }, "kids.did_you_know: frase com 16 palavras (máximo 15)"},
		"descrição com 1 frase":  {func(s *domain.Species) { s.Kids.Description = "O sabiá é laranja." }, "kids.description: precisa de 2 a 4 frases, tem 1"},
		"descrição com 5 frases": {func(s *domain.Species) { s.Kids.Description = strings.Repeat("Ele canta. ", 5) }, "kids.description: precisa de 2 a 4 frases, tem 5"},
		"fato com 2 frases":      {func(s *domain.Species) { s.Kids.Facts[1].Text = "Ele imita. Ele canta." }, "kids.facts[1]: precisa de exatamente 1 frase, tem 2"},
		"proibida com acento":    {func(s *domain.Species) { s.Kids.DidYouKnow = "Ele não pega Doença." }, `kids.did_you_know: palavra proibida "Doença"`},
		"proibida sem acento":    {func(s *domain.Species) { s.Kids.DidYouKnow = "Ele não pega doenca." }, `kids.did_you_know: palavra proibida "doenca"`},
		"número inventado":       {func(s *domain.Species) { s.Kids.Facts[1].Text = "Ele vive três anos." }, "kids.facts[1]: o número 3 não aparece no texto adulto"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := valid()
			tc.change(&s)
			got := errors.Join(kidscontent.Validate(s)...)
			if got == nil || !strings.Contains(got.Error(), "turdus-rufiventris: "+tc.want) {
				t.Fatalf("esperava %q, veio %v", tc.want, got)
			}
		})
	}
}

// 15 palavras passam, e "tico-tico" conta como uma palavra só.
func TestValidate_FifteenWordsWithHyphen(t *testing.T) {
	s := valid()
	s.Kids.DidYouKnow = strings.TrimSpace(strings.Repeat("tico-tico ", 15)) + "."
	if errs := kidscontent.Validate(s); errs != nil {
		t.Fatalf("15 palavras com hífen deveriam passar: %v", errs)
	}
}
