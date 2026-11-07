// Testes do ciclo de geração com uma conversa falsa: a CI nunca chama a API.

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/kidscontent"
)

// fakeConversation devolve as saídas na ordem e guarda as mensagens recebidas.
type fakeConversation struct {
	outputs  []modelOutput
	err      error
	messages []string
}

func (f *fakeConversation) send(_ context.Context, message string) (modelOutput, error) {
	f.messages = append(f.messages, message)
	if f.err != nil {
		return modelOutput{}, f.err
	}
	out := f.outputs[0]
	f.outputs = f.outputs[1:]
	return out, nil
}

// adultSpecies é uma espécie com texto adulto e sem Kids.
func adultSpecies() domain.Species {
	return domain.Species{
		ID: "turdus-rufiventris", ScientificName: "Turdus rufiventris", CommonNamePt: "Sabiá-laranjeira",
		Description: "O sabiá canta por dois minutos. Ele vive em quintais.",
		Facts: []domain.Fact{{Text: "Foi declarado ave nacional em 2002.", Source: "https://a"},
			{Text: "Imita outras aves.", Source: "https://b"}},
		Biomes: []domain.Biome{domain.BiomeMataAtlantica, domain.BiomeCerrado},
		States: []string{"SP"},
	}
}

// goodOutput passa no validador; badOutput tem uma frase de 16 palavras.
func goodOutput() modelOutput {
	return modelOutput{
		Description:  "O sabiá tem a barriga laranja. Ele canta por 2 minutos.",
		Facts:        []factOutput{{Text: "Em 2002 ele virou a ave do Brasil.", SourceFact: 0}},
		DidYouKnow:   "Ele imita outras aves.",
		PrimaryBiome: "mata_atlantica", BiomeReason: "É onde ele é mais lembrado.",
	}
}

func badOutput() modelOutput {
	o := goodOutput()
	o.DidYouKnow = strings.Repeat("pio ", 15) + "pio."
	return o
}

func TestGenerate_AcceptsOnSecondAttempt(t *testing.T) {
	conv := &fakeConversation{outputs: []modelOutput{badOutput(), goodOutput()}}
	res, err := generate(context.Background(), conv, adultSpecies())
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.messages) != 2 || conv.messages[0] != userMessage(adultSpecies()) ||
		!strings.Contains(conv.messages[1], "frase com 16 palavras") {
		t.Fatalf("mensagens: %q", conv.messages)
	}
	k := res.species.Kids
	if res.species.PrimaryBiome != domain.BiomeMataAtlantica || k.GeneratedBy != modelID ||
		k.SourceHash != kidscontent.SourceHash(adultSpecies()) || res.reason == "" {
		t.Fatalf("resultado: %+v %+v", res.species.PrimaryBiome, k)
	}
}

func TestGenerate_GivesUpAfterThirdAttempt(t *testing.T) {
	conv := &fakeConversation{outputs: []modelOutput{badOutput(), badOutput(), badOutput(), goodOutput()}}
	_, err := generate(context.Background(), conv, adultSpecies())
	if err == nil || !strings.Contains(err.Error(), "recusado depois de 3 tentativas") || len(conv.messages) != 3 {
		t.Fatalf("err=%v mensagens=%d", err, len(conv.messages))
	}
}

// Um bioma fora da lista da ave também é recusado e volta como erro ao modelo.
func TestGenerate_BiomeOutsideListIsRetried(t *testing.T) {
	wrong := goodOutput()
	wrong.PrimaryBiome = "pampa"
	conv := &fakeConversation{outputs: []modelOutput{wrong, goodOutput()}}
	if _, err := generate(context.Background(), conv, adultSpecies()); err != nil || !strings.Contains(conv.messages[1], "primary_biome") {
		t.Fatalf("err=%v mensagens=%q", err, conv.messages)
	}
}

// Um erro da API encerra a espécie sem novas tentativas.
func TestGenerate_APIErrorStops(t *testing.T) {
	conv := &fakeConversation{err: errors.New("sem rede")}
	if _, err := generate(context.Background(), conv, adultSpecies()); err == nil || len(conv.messages) != 1 {
		t.Fatalf("err=%v mensagens=%d", err, len(conv.messages))
	}
}
