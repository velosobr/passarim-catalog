// Testa o CONTEÚDO (não o código): garante que todas as aves curadas
// são válidas e têm fonte para cada informação.
package content_test

import (
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/adapter/curated"
)

// notEvaluated lista espécies cujo conservation_status fica vazio de
// propósito: foram recentemente separadas taxonomicamente e a IUCN ainda
// não publicou uma categoria própria para elas (vazio = sem categoria,
// diferente de "DD - Data Deficient", que é uma categoria formal distinta).
var notEvaluated = map[string]bool{
	"troglodytes-musculus": true, // corruíra: separada de Troglodytes aedon, ainda não avaliada pela IUCN.
}

func TestCuratedContent(t *testing.T) {
	species, err := curated.LoadDir("species")
	if err != nil {
		t.Fatal(err)
	}
	if len(species) < 30 {
		t.Fatalf("o MVP precisa de pelo menos 30 aves curadas, há %d", len(species))
	}
	for _, s := range species {
		if len(s.Facts) < 2 {
			t.Errorf("%s: pelo menos 2 curiosidades", s.ID)
		}
		if len(s.Biomes) == 0 || len(s.States) == 0 {
			t.Errorf("%s: biomas e estados são obrigatórios no conteúdo curado", s.ID)
		}
		if len([]rune(s.Description)) < 200 {
			t.Errorf("%s: descrição curta demais (mín. 200 caracteres)", s.ID)
		}
		if !strings.HasPrefix(s.DescriptionCredit.SourceURL, "https://") {
			t.Errorf("%s: description_source_url deve ser https", s.ID)
		}
		if s.ConservationStatus == "" && !notEvaluated[s.ID] {
			t.Errorf("%s: conservation_status obrigatório no conteúdo curado", s.ID)
		}
		for _, f := range s.Facts {
			if !strings.HasPrefix(f.Source, "https://") {
				t.Errorf("%s: fonte da curiosidade deve ser um link https: %q", s.ID, f.Source)
			}
		}
	}
}
