// O ciclo de geração de uma espécie: pede ao modelo, confere com o
// validador do conteúdo e, se ele recusar, devolve os erros ao modelo na
// mesma conversa, até 2 novas tentativas (spec da frente B, seção 3.5). O
// modelo fica atrás da interface conversation: os testes usam um falso, e a
// CI nunca chama a API.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/kidscontent"
)

// modelID é o modelo fixado pelo dono para o kidsgen.
const modelID = "claude-opus-5-5"

// maxAttempts é a primeira tentativa mais as 2 novas.
const maxAttempts = 3

// modelOutput é o que a ferramenta write_kids_content devolve. As tags json
// dizem o nome de cada campo no JSON da ferramenta.
type modelOutput struct {
	Description  string       `json:"description"`
	Facts        []factOutput `json:"facts"`
	DidYouKnow   string       `json:"did_you_know"`
	PrimaryBiome string       `json:"primary_biome"`
	BiomeReason  string       `json:"biome_reason"`
}

type factOutput struct {
	Text       string `json:"text"`
	SourceFact int    `json:"source_fact"`
}

// conversation é uma conversa com o modelo sobre UMA espécie. send manda a
// mensagem (na primeira vez, o texto adulto; depois, os erros do validador)
// e devolve a saída da ferramenta.
type conversation interface {
	send(ctx context.Context, message string) (modelOutput, error)
}

// result é uma geração aceita pelo validador.
type result struct {
	species domain.Species // a espécie com o PrimaryBiome e o Kids novos
	reason  string         // a justificativa do bioma, só para o terminal
}

// generate roda o ciclo para uma espécie. Um erro da API encerra a espécie
// na hora (tentar de novo não corrige uma chave errada nem a falta de
// rede); só a recusa do validador gasta as novas tentativas.
func generate(ctx context.Context, conv conversation, s domain.Species) (result, error) {
	message := userMessage(s)
	var problems []error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		out, err := conv.send(ctx, message)
		if err != nil {
			return result{}, fmt.Errorf("tentativa %d: %w", attempt, err)
		}
		candidate := withKids(s, out)
		if problems = kidscontent.Validate(candidate); len(problems) == 0 {
			return result{species: candidate, reason: out.BiomeReason}, nil
		}
		message = feedback(problems)
	}
	return result{}, fmt.Errorf("recusado depois de %d tentativas: %w", maxAttempts, errors.Join(problems...))
}

// withKids devolve uma cópia da espécie com a saída do modelo. generated_by
// e source_hash são preenchidos aqui, não pelo modelo.
func withKids(s domain.Species, out modelOutput) domain.Species {
	k := &domain.KidsContent{
		Description: kidscontent.Collapse(out.Description),
		DidYouKnow:  kidscontent.Collapse(out.DidYouKnow),
		GeneratedBy: modelID,
		SourceHash:  kidscontent.SourceHash(s),
	}
	for _, f := range out.Facts {
		k.Facts = append(k.Facts, domain.KidsFact{Text: kidscontent.Collapse(f.Text), SourceFact: f.SourceFact})
	}
	s.PrimaryBiome = domain.Biome(out.PrimaryBiome)
	s.Kids = k
	return s
}

// feedback é a mensagem com os erros do validador para a nova tentativa.
func feedback(problems []error) string {
	var b strings.Builder
	b.WriteString("O validador recusou o conteúdo:\n")
	for _, p := range problems {
		b.WriteString("- " + p.Error() + "\n")
	}
	b.WriteString("Corrija só esses pontos e chame write_kids_content de novo com o conteúdo inteiro.")
	return b.String()
}
