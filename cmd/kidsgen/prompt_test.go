// Testes do prompt: a mensagem da espécie com um golden file e as regras
// fixas do prompt.md.

package main

import (
	"flag"
	"os"
	"strings"
	"testing"
)

// update regrava o golden file: go test ./cmd/kidsgen/ -run TestUserMessageGolden -update
var update = flag.Bool("update", false, "regrava os golden files em testdata/")

func TestUserMessageGolden(t *testing.T) {
	got := userMessage(adultSpecies())
	golden := "testdata/user_message.golden"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden ausente (rode com -update): %v", err)
	}
	if got != string(want) {
		t.Fatalf("mensagem mudou:\n%s\n--- golden ---\n%s", got, want)
	}
	// O que nunca vai ao modelo: links, UFs e o nome científico.
	for _, leak := range []string{"https://", "Turdus rufiventris", "SP"} {
		if strings.Contains(got, leak) {
			t.Errorf("a mensagem não deveria ter %q", leak)
		}
	}
}

// As regras do prompt citam os limites da spec.
func TestSystemPromptRules(t *testing.T) {
	for _, rule := range []string{"15 palavras", "de 2 a 4 frases", "write_kids_content", "Suavizar sem esconder", "carcaça"} {
		if !strings.Contains(strings.ToLower(systemPrompt), strings.ToLower(rule)) {
			t.Errorf("o prompt deveria citar %q", rule)
		}
	}
}
