// Teste do esquema JSON da resposta do modelo. A conversa com a API em si
// não é testada: a CI nunca chama a API (spec da frente B, seção 9).

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// O bioma principal só pode sair da lista da ave, todos os campos são
// obrigatórios e nenhum campo a mais é aceito (additionalProperties: false,
// que a saída estruturada exige).
func TestOutputSchema(t *testing.T) {
	raw, err := json.Marshal(outputSchema([]string{"mata_atlantica", "cerrado"}))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Enum  []string `json:"enum"`
			Items struct {
				AdditionalProperties *bool `json:"additionalProperties"`
			} `json:"items"`
		} `json:"properties"`
		Required             []string `json:"required"`
		AdditionalProperties *bool    `json:"additionalProperties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(schema.Properties["primary_biome"].Enum, ","); got != "mata_atlantica,cerrado" {
		t.Fatalf("enum = %q", got)
	}
	if len(schema.Required) != 5 || schema.AdditionalProperties == nil || *schema.AdditionalProperties ||
		schema.Properties["facts"].Items.AdditionalProperties == nil || *schema.Properties["facts"].Items.AdditionalProperties {
		t.Fatalf("esquema: %s", raw)
	}
}
