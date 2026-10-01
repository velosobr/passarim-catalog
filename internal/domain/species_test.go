package domain_test

import (
	"errors"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

func TestSpeciesID(t *testing.T) {
	cases := map[string]string{
		"Turdus rufiventris":         "turdus-rufiventris",
		"  Ramphastos   toco  ":      "ramphastos-toco",
		"Anodorhynchus Hyacinthinus": "anodorhynchus-hyacinthinus",
	}
	for in, want := range cases {
		if got := domain.SpeciesID(in); got != want {
			t.Errorf("SpeciesID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeForSearch(t *testing.T) {
	cases := map[string]string{
		"Sabiá-Laranjeira": "sabia-laranjeira",
		"  JOÃO-DE-BARRO ": "joao-de-barro",
		"Tiê-sangue":       "tie-sangue",
		"Saíra-sete-cores": "saira-sete-cores",
	}
	for in, want := range cases {
		if got := domain.NormalizeForSearch(in); got != want {
			t.Errorf("NormalizeForSearch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseBiome(t *testing.T) {
	if b, err := domain.ParseBiome("pantanal"); err != nil || b != domain.BiomePantanal {
		t.Fatalf("ParseBiome(pantanal) = %v, %v", b, err)
	}
	_, err := domain.ParseBiome("deserto")
	var inv *domain.InvalidArgumentError
	if !errors.As(err, &inv) || inv.Field != "biome" {
		t.Fatalf("esperava InvalidArgumentError em biome, veio %v", err)
	}
	if len(domain.AllBiomes()) != 6 {
		t.Fatalf("esperava 6 biomas")
	}
}

func TestParseConservationStatus(t *testing.T) {
	for _, s := range []string{"", "LC", "NT", "VU", "EN", "CR", "EW", "EX", "DD"} {
		if _, err := domain.ParseConservationStatus(s); err != nil {
			t.Errorf("%q deveria ser válido: %v", s, err)
		}
	}
	if _, err := domain.ParseConservationStatus("XX"); err == nil {
		t.Error("XX deveria ser inválido")
	}
}

func TestNormalizeUF(t *testing.T) {
	if uf, err := domain.NormalizeUF(" ms "); err != nil || uf != "MS" {
		t.Fatalf("NormalizeUF(ms) = %q, %v", uf, err)
	}
	if _, err := domain.NormalizeUF("XX"); err == nil {
		t.Fatal("XX não é UF")
	}
	if len(domain.AllUFs()) != 27 {
		t.Fatalf("esperava 27 UFs (26 estados + DF), veio %d", len(domain.AllUFs()))
	}
}

func validSpecies() domain.Species {
	size := 25
	return domain.Species{
		ID: "turdus-rufiventris", ScientificName: "Turdus rufiventris",
		CommonNamePt: "Sabiá-laranjeira", Family: "Turdidae", SizeCm: &size,
		ConservationStatus: domain.StatusLC,
		Biomes:             []domain.Biome{domain.BiomeMataAtlantica},
		States:             []string{"SP"},
		Facts:              []domain.Fact{{Text: "É a ave-símbolo do Brasil.", Source: "https://pt.wikipedia.org/wiki/Sabi%C3%A1-laranjeira"}},
	}
}

func TestSpeciesValidate(t *testing.T) {
	if err := validSpecies().Validate(); err != nil {
		t.Fatalf("espécie válida recusada: %v", err)
	}
	broken := map[string]func(*domain.Species){
		"scientific_name": func(s *domain.Species) { s.ScientificName = "Turdus" },
		"common_name_pt":  func(s *domain.Species) { s.CommonNamePt = " " },
		"family":          func(s *domain.Species) { s.Family = "" },
		"size_cm":         func(s *domain.Species) { z := 0; s.SizeCm = &z },
		"biomes":          func(s *domain.Species) { s.Biomes = []domain.Biome{"deserto"} },
		"states":          func(s *domain.Species) { s.States = []string{"sp"} },
		"facts":           func(s *domain.Species) { s.Facts[0].Source = "" },
		"id":              func(s *domain.Species) { s.ID = "outro-id" },
	}
	for field, breakIt := range broken {
		s := validSpecies()
		breakIt(&s)
		err := s.Validate()
		var inv *domain.InvalidArgumentError
		if !errors.As(err, &inv) || inv.Field != field {
			t.Errorf("campo %s: esperava InvalidArgumentError, veio %v", field, err)
		}
	}
}
