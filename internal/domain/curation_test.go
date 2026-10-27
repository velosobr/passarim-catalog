// Testes das regras do override de fotos (a chave photos: do YAML).

package domain_test

import (
	"errors"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// Cada caso diz se o override é aceito; os recusados dizem o campo do erro.
func TestPhotoOverrideValidate(t *testing.T) {
	cases := []struct {
		name  string
		o     domain.PhotoOverride
		field string // "" = válido
	}{
		{"só principal", domain.PhotoOverride{PrimaryObservationID: "123456", PrimaryPhotoID: "219317614"}, ""},
		{"só exclusões", domain.PhotoOverride{ExcludedIDs: []string{"1", "2"}}, ""},
		{"vazio", domain.PhotoOverride{}, "photos"},
		{"principal sem observação", domain.PhotoOverride{PrimaryPhotoID: "1"}, "photos.primary"},
		{"principal sem foto", domain.PhotoOverride{PrimaryObservationID: "1"}, "photos.primary"},
		{"id com letra", domain.PhotoOverride{PrimaryObservationID: "12a", PrimaryPhotoID: "1"}, "photos.primary"},
		{"id zero", domain.PhotoOverride{PrimaryObservationID: "0", PrimaryPhotoID: "1"}, "photos.primary"},
		{"exclusão inválida", domain.PhotoOverride{ExcludedIDs: []string{"../x"}}, "photos.exclude"},
		{"principal excluída", domain.PhotoOverride{PrimaryObservationID: "1", PrimaryPhotoID: "7", ExcludedIDs: []string{"7"}}, "photos.exclude"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.o.Validate()
			if c.field == "" {
				if err != nil {
					t.Fatalf("esperava válido: %v", err)
				}
				return
			}
			var inv *domain.InvalidArgumentError
			if !errors.As(err, &inv) || inv.Field != c.field {
				t.Fatalf("esperava erro no campo %q, veio %v", c.field, err)
			}
		})
	}
}

// Excludes e HasPrimary respondem pelo que o YAML disse.
func TestPhotoOverrideHelpers(t *testing.T) {
	o := domain.PhotoOverride{PrimaryObservationID: "1", PrimaryPhotoID: "2", ExcludedIDs: []string{"3"}}
	if !o.HasPrimary() || !o.Excludes("3") || o.Excludes("2") {
		t.Fatalf("helpers errados: %+v", o)
	}
	if (domain.PhotoOverride{}).HasPrimary() {
		t.Fatal("override vazio não tem principal")
	}
}

// Uma espécie com override inválido é recusada pelo Validate da espécie.
func TestSpeciesValidateChecksOverride(t *testing.T) {
	s := validSpecies()
	s.PhotoOverride = &domain.PhotoOverride{}
	var inv *domain.InvalidArgumentError
	if err := s.Validate(); !errors.As(err, &inv) || inv.Field != "photos" {
		t.Fatalf("esperava erro em photos, veio %v", err)
	}
}
