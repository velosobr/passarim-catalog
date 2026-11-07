// Teste do esquema da ferramenta write_kids_content. A conversa com a API
// em si não é testada: a CI nunca chama a API (spec da frente B, seção 9).

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// O bioma principal só pode sair da lista da ave, e todos os campos são obrigatórios.
func TestToolSchema(t *testing.T) {
	raw, err := json.Marshal(toolSchema([]string{"mata_atlantica", "cerrado"}))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(schema.Properties["primary_biome"].Enum, ","); got != "mata_atlantica,cerrado" {
		t.Fatalf("enum = %q", got)
	}
	if len(schema.Required) != 5 {
		t.Fatalf("required = %v", schema.Required)
	}
}
