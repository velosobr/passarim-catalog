// Testes do carregador de conteúdo curado, com arquivos de exemplo em
// testdata/.

package curated_test

import (
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/adapter/curated"
	"github.com/velosobr/passarim-catalog/internal/domain"
)

// Uma pasta válida vira as espécies, com o id derivado e os textos limpos.
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

// Campo com erro de digitação não pode passar em silêncio.
func TestLoadDir_UnknownFieldFails(t *testing.T) {
	_, err := curated.LoadDir("testdata/typo")
	if err == nil || !strings.Contains(err.Error(), "bad.yaml") || !strings.Contains(err.Error(), "familly") {
		t.Fatalf("esperava erro citando arquivo e campo, veio %v", err)
	}
}

// Um arquivo com dado inválido (bioma inexistente) faz a pasta inteira
// falhar, citando o arquivo.
func TestLoadDir_InvalidSpeciesFails(t *testing.T) {
	_, err := curated.LoadDir("testdata/invalid-biome")
	if err == nil || !strings.Contains(err.Error(), "bad.yaml") || !strings.Contains(err.Error(), "biomes") {
		t.Fatalf("esperava erro de validação citando o arquivo, veio %v", err)
	}
}

// Uma pasta sem YAML é erro.
func TestLoadDir_MissingDirFails(t *testing.T) {
	if _, err := curated.LoadDir("testdata/nao-existe"); err == nil {
		t.Fatal("pasta inexistente deveria dar erro")
	}
}

// A chave photos: vira o override da espécie, com os ids como texto.
func TestLoadDir_PhotosOverride(t *testing.T) {
	all, err := curated.LoadDir("testdata/photos")
	if err != nil {
		t.Fatal(err)
	}
	o := all[0].PhotoOverride
	if o == nil || o.PrimaryObservationID != "123456" || o.PrimaryPhotoID != "219317614" ||
		len(o.ExcludedIDs) != 2 || o.ExcludedIDs[1] != "219317616" {
		t.Fatalf("override lido errado: %+v", o)
	}
}

// Sem a chave photos:, não há override.
func TestLoadDir_NoPhotosKeyMeansNoOverride(t *testing.T) {
	all, err := curated.LoadDir("testdata/valid")
	if err != nil || all[0].PhotoOverride != nil {
		t.Fatalf("esperava sem override: %+v %v", all[0].PhotoOverride, err)
	}
}

// Chave errada dentro de photos: também é recusada (KnownFields).
func TestLoadDir_PhotosTypoIsError(t *testing.T) {
	if _, err := curated.LoadDir("testdata/photos-typo"); err == nil || !strings.Contains(err.Error(), "primari") {
		t.Fatalf("esperava erro citando a chave errada, veio %v", err)
	}
}

// A principal que também está excluída é recusada.
func TestLoadDir_PrimaryExcludedIsError(t *testing.T) {
	if _, err := curated.LoadDir("testdata/photos-conflict"); err == nil || !strings.Contains(err.Error(), "photos.exclude") {
		t.Fatalf("esperava erro em photos.exclude, veio %v", err)
	}
}
