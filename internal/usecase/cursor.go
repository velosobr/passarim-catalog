package usecase

import (
	"encoding/base64"
	"encoding/json"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// Cursor marca "onde a página anterior parou". Usamos paginação por
// cursor (e não "página 3") porque ela não pula nem repete itens quando
// novas aves são inseridas entre uma página e outra.
type Cursor struct {
	SortName string `json:"s"` // nome popular normalizado do último item
	ID       string `json:"i"` // desempate: dois nomes iguais têm ids diferentes
}

// EncodeCursor transforma o cursor num texto "opaco" para o cliente.
// Opaco = o cliente não deve interpretar nem montar esse texto, só devolvê-lo.
func EncodeCursor(c Cursor) string {
	b, _ := json.Marshal(c) // struct com 2 strings: Marshal nunca falha
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor faz o caminho inverso e recusa tokens adulterados.
func DecodeCursor(token string) (Cursor, error) {
	bad := &domain.InvalidArgumentError{Field: "page_token", Reason: "token de página inválido"}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, bad
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil || c.ID == "" {
		return Cursor{}, bad
	}
	return c, nil
}
