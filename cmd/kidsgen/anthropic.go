// A conversa de verdade com o modelo, pelo SDK oficial da Anthropic para Go
// (github.com/anthropics/anthropic-sdk-go). É o único arquivo do kidsgen
// que conhece o SDK; o resto conversa pela interface conversation.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// callTimeout é o prazo de cada chamada à API (spec, seção 3.5).
const callTimeout = 2 * time.Minute

// maxTokens é o teto de tokens da resposta. O claude-opus-5-5 sempre pensa
// antes de responder, e o raciocínio conta nesse teto: com pouco espaço, a
// resposta pararia no meio do JSON.
const maxTokens = 16000

// anthropicConversation guarda o histórico da conversa sobre uma espécie:
// a API não guarda nada entre chamadas, então cada nova tentativa manda as
// mensagens anteriores de novo, com a resposta do modelo e os erros do
// validador.
type anthropicConversation struct {
	client   anthropic.Client
	biomes   []string
	messages []anthropic.MessageParam
}

// newAnthropicConversation abre uma conversa para a espécie.
func newAnthropicConversation(apiKey string, s domain.Species) conversation {
	return &anthropicConversation{client: anthropic.NewClient(option.WithAPIKey(apiKey)), biomes: biomeNames(s)}
}

// send pede a resposta em "saída estruturada" (output_config.format): a API
// garante que o texto devolvido é um JSON que segue o esquema de
// outputSchema. É assim que a resposta vem com campos fixos, em vez de texto
// solto.
func (c *anthropicConversation) send(ctx context.Context, message string) (modelOutput, error) {
	c.messages = append(c.messages, anthropic.NewUserMessage(anthropic.NewTextBlock(message)))
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(modelID),
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:  c.messages,
		OutputConfig: anthropic.OutputConfigParam{
			// effort é o quanto o modelo raciocina; fixado aqui para o custo
			// de uma rodada não mudar se o padrão da API mudar.
			Effort: anthropic.OutputConfigEffortMedium,
			Format: anthropic.JSONOutputFormatParam{Schema: outputSchema(c.biomes)},
		},
	})
	if err != nil {
		return modelOutput{}, err
	}
	// stop_reason diz por que a resposta acabou. Só "end_turn" é uma resposta
	// inteira; "max_tokens" ou "refusal" deixariam o JSON pela metade.
	if resp.StopReason != anthropic.StopReasonEndTurn {
		return modelOutput{}, fmt.Errorf("a resposta parou antes do fim (stop_reason %s)", resp.StopReason)
	}
	c.messages = append(c.messages, resp.ToParam())
	// A resposta pode ter blocos de raciocínio antes do texto: só o texto é o JSON.
	var text strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	var out modelOutput
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return modelOutput{}, fmt.Errorf("resposta fora do formato JSON: %w", err)
	}
	return out, nil
}

// outputSchema é o esquema JSON da resposta. primary_biome é um enum com os
// biomas da ave: o modelo não consegue escolher um bioma fora da lista.
// additionalProperties: false (nenhum campo além dos listados) é exigido
// pela saída estruturada.
func outputSchema(biomes []string) map[string]any {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"description": str("De 2 a 4 frases, cada uma com até 15 palavras."),
			"facts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text":        str("Uma frase com até 15 palavras."),
						"source_fact": map[string]any{"type": "integer", "description": "O número do fato adulto de origem."},
					},
					"required":             []string{"text", "source_fact"},
					"additionalProperties": false,
				},
			},
			"did_you_know":  str("Uma frase com até 15 palavras."),
			"primary_biome": map[string]any{"type": "string", "enum": biomes},
			"biome_reason":  str("Uma frase explicando a escolha do bioma, para quem revisa."),
		},
		"required":             []string{"description", "facts", "did_you_know", "primary_biome", "biome_reason"},
		"additionalProperties": false,
	}
}
