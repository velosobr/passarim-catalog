// Formato JSON da coluna species.kids (jsonb). As tags `json:"..."` ficam
// aqui no adapter, e não no domínio, que não sabe de JSON. Só vai para o
// banco o que o app mostra: generated_by e source_hash ficam no YAML.

package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// kidsDTO e kidsFactDTO espelham o JSON gravado.
type kidsDTO struct {
	Description string        `json:"description"`
	Facts       []kidsFactDTO `json:"facts"`
	DidYouKnow  string        `json:"did_you_know"`
}

type kidsFactDTO struct {
	Text       string `json:"text"`
	SourceFact int    `json:"source_fact"`
}

// kidsToJSON converte o conteúdo Kids no JSON da coluna. nil vira nil, que o
// pgx grava como NULL (espécie sem conteúdo Kids).
func kidsToJSON(k *domain.KidsContent) ([]byte, error) {
	if k == nil {
		return nil, nil
	}
	dto := kidsDTO{Description: k.Description, DidYouKnow: k.DidYouKnow, Facts: make([]kidsFactDTO, len(k.Facts))}
	for i, f := range k.Facts {
		dto.Facts[i] = kidsFactDTO{Text: f.Text, SourceFact: f.SourceFact}
	}
	return json.Marshal(dto)
}

// kidsFromJSON faz o caminho inverso. NULL (nil) vira nil.
func kidsFromJSON(raw []byte) (*domain.KidsContent, error) {
	if raw == nil {
		return nil, nil
	}
	var dto kidsDTO
	if err := json.Unmarshal(raw, &dto); err != nil {
		return nil, fmt.Errorf("conteúdo kids inválido no banco: %w", err)
	}
	out := &domain.KidsContent{Description: dto.Description, DidYouKnow: dto.DidYouKnow}
	for _, f := range dto.Facts {
		out.Facts = append(out.Facts, domain.KidsFact{Text: f.Text, SourceFact: f.SourceFact})
	}
	return out, nil
}
