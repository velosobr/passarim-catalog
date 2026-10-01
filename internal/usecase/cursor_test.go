package usecase_test

import (
	"errors"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

func TestCursorRoundTrip(t *testing.T) {
	c := usecase.Cursor{SortName: "sabia-laranjeira", ID: "turdus-rufiventris"}
	got, err := usecase.DecodeCursor(usecase.EncodeCursor(c))
	if err != nil || got != c {
		t.Fatalf("ida e volta falhou: %v, %v", got, err)
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	for _, token := range []string{"!!!", "e30", "bm90LWpzb24"} { // inválido, "{}", "not-json"
		_, err := usecase.DecodeCursor(token)
		var inv *domain.InvalidArgumentError
		if !errors.As(err, &inv) || inv.Field != "page_token" {
			t.Errorf("token %q: esperava InvalidArgumentError(page_token), veio %v", token, err)
		}
	}
}
