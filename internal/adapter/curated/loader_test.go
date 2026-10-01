package curated_test

import (
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/adapter/curated"
	"github.com/velosobr/passarim-catalog/internal/domain"
)

func TestLoadDir_Valid(t *testing.T) {
	species, err := curated.LoadDir("testdata/valid")
	if err != nil {
		t.Fatal(err)
	}
	if len(species) != 1 {
		t.Fatalf("esperava 1 espécie, veio %d", len(species))
	}
	s := species[0]
	if s.ID != "turdus-rufiventris" || s.CommonNamePt != "Sabiá-laranjeira" || *s.SizeCm != 25 {
		t.Fatalf("campos errados: %+v", s)
	}
	if s.ConservationStatus != domain.StatusLC || len(s.Biomes) != 3 || len(s.Facts) != 1 {
		t.Fatalf("listas erradas: %+v", s)
	}
	want := domain.Credit{Author: "Equipe Passarim", License: "CC-BY-4.0", Source: "curated", SourceURL: "https://pt.wikipedia.org/wiki/Sabi%C3%A1-laranjeira"}
	if s.DescriptionCredit != want {
		t.Fatalf("crédito = %+v", s.DescriptionCredit)
	}
	if !strings.HasPrefix(s.Description, "Ave de peito") {
		t.Fatalf("descrição = %q", s.Description)
	}
}

// Review Focus #5: campo com erro de digitação não pode passar em silêncio.
func TestLoadDir_UnknownFieldFails(t *testing.T) {
	_, err := curated.LoadDir("testdata/typo")
	if err == nil || !strings.Contains(err.Error(), "bad.yaml") || !strings.Contains(err.Error(), "familly") {
		t.Fatalf("esperava erro citando arquivo e campo, veio %v", err)
	}
}

func TestLoadDir_InvalidSpeciesFails(t *testing.T) {
	_, err := curated.LoadDir("testdata/invalid-biome")
	if err == nil || !strings.Contains(err.Error(), "bad.yaml") || !strings.Contains(err.Error(), "biomes") {
		t.Fatalf("esperava erro de validação citando o arquivo, veio %v", err)
	}
}

func TestLoadDir_MissingDirFails(t *testing.T) {
	if _, err := curated.LoadDir("testdata/nao-existe"); err == nil {
		t.Fatal("pasta inexistente deveria dar erro")
	}
}
