// A conversa de verdade com o modelo, pelo SDK oficial da Anthropic para Go
// (github.com/anthropics/anthropic-sdk-go). É o único arquivo do kidsgen
// que conhece o SDK; o resto conversa pela interface conversation.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// toolName é a ferramenta que o modelo é obrigado a chamar: é assim que a
// resposta vem como JSON com um esquema fixo, em vez de texto solto.
const toolName = "write_kids_content"

// callTimeout é o prazo de cada chamada à API (spec, seção 3.5).
const callTimeout = 2 * time.Minute

// anthropicConversation guarda o histórico da conversa sobre uma espécie:
// a API não guarda nada entre chamadas, então cada nova tentativa manda as
// mensagens anteriores de novo.
type anthropicConversation struct {
	client        anthropic.Client
	biomes        []string
	messages      []anthropic.MessageParam
	lastToolUseID string // o id da última chamada da ferramenta
}

// newAnthropicConversation abre uma conversa para a espécie.
func newAnthropicConversation(apiKey string, s domain.Species) conversation {
	return &anthropicConversation{client: anthropic.NewClient(option.WithAPIKey(apiKey)), biomes: biomeNames(s)}
}

func (c *anthropicConversation) send(ctx context.Context, message string) (modelOutput, error) {
	if c.lastToolUseID == "" {
		c.messages = append(c.messages, anthropic.NewUserMessage(anthropic.NewTextBlock(message)))
	} else {
		// Depois de uma chamada de ferramenta, a API exige que a mensagem
		// seguinte traga o resultado dela (tool_result). É por ele que vão
		// os erros do validador, marcados como erro (isError = true).
		c.messages = append(c.messages, anthropic.NewUserMessage(anthropic.NewToolResultBlock(c.lastToolUseID, message, true)))
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(modelID),
		MaxTokens: 2048,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:  c.messages,
		Tools:     []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema(c.biomes), toolName)},
		// ToolChoice força o modelo a responder chamando a ferramenta.
		ToolChoice: anthropic.ToolChoiceParamOfTool(toolName),
	})
	if err != nil {
		return modelOutput{}, err
	}
	c.messages = append(c.messages, resp.ToParam())
	for _, block := range resp.Content {
		if block.Type == "tool_use" && block.Name == toolName {
			c.lastToolUseID = block.ID
			var out modelOutput
			if err := json.Unmarshal(block.Input, &out); err != nil {
				return modelOutput{}, fmt.Errorf("saída da ferramenta inválida: %w", err)
			}
			return out, nil
		}
	}
	return modelOutput{}, errors.New("o modelo não chamou a ferramenta " + toolName)
}

// toolSchema é o esquema JSON da ferramenta. primary_biome é um enum com os
// biomas da ave: o modelo não consegue escolher um bioma fora da lista.
func toolSchema(biomes []string) anthropic.ToolInputSchemaParam {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"description": str("De 2 a 4 frases, cada uma com até 15 palavras."),
			"facts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text":        str("Uma frase com até 15 palavras."),
						"source_fact": map[string]any{"type": "integer", "minimum": 0, "description": "O número do fato adulto de origem."},
					},
					"required": []string{"text", "source_fact"},
				},
			},
			"did_you_know":  str("Uma frase com até 15 palavras."),
			"primary_biome": map[string]any{"type": "string", "enum": biomes},
			"biome_reason":  str("Uma frase explicando a escolha do bioma, para quem revisa."),
		},
		Required: []string{"description", "facts", "did_you_know", "primary_biome", "biome_reason"},
	}
}
