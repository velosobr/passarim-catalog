// Testes do parser de números com um golden file: testdata/numbers.json
// tem frases reais e os valores que o parser deve achar nelas.

package kidscontent_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/kidscontent"
)

// numberCase é uma linha do golden file.
type numberCase struct {
	Text string    `json:"text"`
	Want []float64 `json:"want"`
}

func TestNumbersGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/numbers.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []numberCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := kidscontent.Numbers(c.Text); fmt.Sprint(got) != fmt.Sprint(c.Want) {
			t.Errorf("%q: veio %v, esperava %v", c.Text, got, c.Want)
		}
	}
}
