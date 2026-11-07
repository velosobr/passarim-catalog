// Testes das regras do bioma principal e do bloco Kids na validação da
// espécie. As regras de texto (tamanho das frases, números, hash) ficam no
// validador do conteúdo, no plano B2; aqui só a estrutura.

package domain_test

import (
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// kidsSpecies devolve uma espécie válida com bloco Kids completo.
func kidsSpecies() domain.Species {
	return domain.Species{
		ID: "turdus-rufiventris", ScientificName: "Turdus rufiventris", CommonNamePt: "Sabiá-laranjeira", Family: "Turdidae",
		Biomes:       []domain.Biome{domain.BiomeMataAtlantica, domain.BiomeCerrado},
		Facts:        []domain.Fact{{Text: "Fato adulto 0", Source: "https://a"}, {Text: "Fato adulto 1", Source: "https://b"}},
		PrimaryBiome: domain.BiomeMataAtlantica,
		Kids: &domain.KidsContent{Description: "O sabiá tem a barriga laranja.", DidYouKnow: "Ele imita outras aves.",
			Facts: []domain.KidsFact{{Text: "Ele canta muito.", SourceFact: 1}}, GeneratedBy: "claude-opus-5-5", SourceHash: "sha256:abc"},
	}
}

func TestValidate_KidsValid(t *testing.T) {
	if err := kidsSpecies().Validate(); err != nil {
		t.Fatalf("espécie válida recusada: %v", err)
	}
}

// Cada caso quebra uma regra e confere o campo citado no erro.
func TestValidate_KidsRules(t *testing.T) {
	cases := map[string]struct {
		change func(*domain.Species)
		field  string
	}{
		"bioma principal fora dos biomas": {func(s *domain.Species) { s.PrimaryBiome = domain.BiomePampa }, "primary_biome"},
		"bioma principal desconhecido":    {func(s *domain.Species) { s.PrimaryBiome = "lua" }, "primary_biome"},
		"kids sem bioma principal":        {func(s *domain.Species) { s.PrimaryBiome = "" }, "primary_biome"},
		"kids sem descrição":              {func(s *domain.Species) { s.Kids.Description = " " }, "kids"},
		"kids sem você sabia":             {func(s *domain.Species) { s.Kids.DidYouKnow = "" }, "kids"},
		"kids sem generated_by":           {func(s *domain.Species) { s.Kids.GeneratedBy = "" }, "kids"},
		"kids sem source_hash":            {func(s *domain.Species) { s.Kids.SourceHash = "" }, "kids"},
		"kids sem fatos":                  {func(s *domain.Species) { s.Kids.Facts = nil }, "kids.facts"},
		"fato kids sem texto":             {func(s *domain.Species) { s.Kids.Facts[0].Text = "" }, "kids.facts"},
		"fato de origem inexistente":      {func(s *domain.Species) { s.Kids.Facts[0].SourceFact = 2 }, "kids.facts"},
		"fato de origem negativo":         {func(s *domain.Species) { s.Kids.Facts[0].SourceFact = -1 }, "kids.facts"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := kidsSpecies()
			tc.change(&s)
			err := s.Validate()
			if err == nil || !strings.HasPrefix(err.Error(), tc.field+":") {
				t.Fatalf("esperava erro no campo %q, veio %v", tc.field, err)
			}
		})
	}
}

// Sem bloco Kids, o bioma principal é opcional, mas se vier precisa estar
// entre os biomas (o loader recusa mesmo sem o bloco).
func TestValidate_PrimaryBiomeWithoutKids(t *testing.T) {
	s := kidsSpecies()
	s.Kids = nil
	if err := s.Validate(); err != nil {
		t.Fatalf("bioma principal sem Kids deveria passar: %v", err)
	}
	s.PrimaryBiome = ""
	if err := s.Validate(); err != nil {
		t.Fatalf("sem bioma principal e sem Kids deveria passar: %v", err)
	}
	s.PrimaryBiome = domain.BiomePampa
	if err := s.Validate(); err == nil {
		t.Fatal("bioma principal fora dos biomas deveria falhar mesmo sem Kids")
	}
}
