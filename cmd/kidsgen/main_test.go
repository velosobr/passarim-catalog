// Testes do programa inteiro (run) numa pasta temporária, com a conversa
// falsa no lugar do modelo.

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/adapter/curated"
	"github.com/velosobr/passarim-catalog/internal/domain"
)

// noFactsYAML é uma espécie sem fatos adultos: o kidsgen a pula.
const noFactsYAML = `scientific_name: Crotophaga ani
common_name_pt: Anu-preto
family: Cuculidae
conservation_status: LC
biomes: [cerrado]
states: [SP]
description: >
  Ave preta de bico alto.
description_source_url: https://x
`

// contentDir monta uma pasta com o sabiá (de baseYAML) e o anu sem fatos.
func contentDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{"turdus-rufiventris.yaml": baseYAML, "crotophaga-ani.yaml": noFactsYAML} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fakeStarter abre sempre a mesma conversa falsa e conta quantas abriu.
func fakeStarter(conv *fakeConversation, opened *int) starter {
	return func(string, domain.Species) conversation {
		*opened++
		return conv
	}
}

// O sabiá precisa de saídas que casem com os fatos de baseYAML.
func sabiaOutput() modelOutput {
	return modelOutput{
		Description:  "O sabiá tem o peito laranja. Ele vive em quintais.",
		Facts:        []factOutput{{Text: "Em 2002 ele virou ave-símbolo.", SourceFact: 0}},
		DidYouKnow:   "Ele imita outras aves.",
		PrimaryBiome: "mata_atlantica", BiomeReason: "É a mata dos quintais do Sudeste.",
	}
}

func TestRun_NoKeyFailsBeforeCalling(t *testing.T) {
	opened := 0
	err := run(context.Background(), []string{"--dir", contentDir(t), "--missing"}, "", &bytes.Buffer{}, fakeStarter(nil, &opened))
	if err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") || opened != 0 {
		t.Fatalf("err=%v conversas=%d", err, opened)
	}
}

// --dry-run sem chave só mostra o prompt e não grava nada.
func TestRun_DryRunWithoutKeyShowsPrompt(t *testing.T) {
	dir := contentDir(t)
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--dir", dir, "--ids", "turdus-rufiventris", "--dry-run"}, "", &out, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "== prompt do sistema ==") || !strings.Contains(out.String(), "Ave: Sabiá-laranjeira") {
		t.Fatalf("saída:\n%s", out.String())
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "turdus-rufiventris.yaml")); string(raw) != baseYAML { //nolint:gosec // G304: pasta temporária do teste
		t.Fatal("o --dry-run não deveria gravar")
	}
}

// --missing gera o sabiá, grava o YAML (lido de volta pelo loader) e pula o
// anu sem chamar o modelo.
func TestRun_MissingWritesAndSkipsWithoutAdultText(t *testing.T) {
	dir := contentDir(t)
	conv := &fakeConversation{outputs: []modelOutput{sabiaOutput()}}
	opened := 0
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--dir", dir, "--missing"}, "chave", &out, fakeStarter(conv, &opened)); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if opened != 1 || !strings.Contains(out.String(), "crotophaga-ani: pulada, sem texto adulto curado") ||
		!strings.Contains(out.String(), "turdus-rufiventris → primary_biome: mata_atlantica (É a mata dos quintais do Sudeste.)") {
		t.Fatalf("conversas=%d saída:\n%s", opened, out.String())
	}
	species, err := curated.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if species[1].ID != "turdus-rufiventris" || species[1].Kids == nil || species[1].PrimaryBiome != domain.BiomeMataAtlantica {
		t.Fatalf("o YAML não ganhou o bloco: %+v", species[1])
	}
	// Uma segunda rodada --missing não acha mais o sabiá: o carimbo bate.
	opened = 0
	if err := run(context.Background(), []string{"--dir", dir, "--missing"}, "chave", &bytes.Buffer{}, fakeStarter(conv, &opened)); err != nil || opened != 0 {
		t.Fatalf("err=%v conversas=%d", err, opened)
	}
}

// Uma espécie recusada nas 3 tentativas não é gravada, e run devolve erro
// (o programa sai com código 1) citando a espécie.
func TestRun_FailureIsReported(t *testing.T) {
	dir := contentDir(t)
	bad := sabiaOutput()
	bad.PrimaryBiome = "pampa"
	conv := &fakeConversation{outputs: []modelOutput{bad, bad, bad}}
	opened := 0
	err := run(context.Background(), []string{"--dir", dir, "--ids", "turdus-rufiventris"}, "chave", &bytes.Buffer{}, fakeStarter(conv, &opened))
	if err == nil || !strings.Contains(err.Error(), "turdus-rufiventris") {
		t.Fatalf("esperava erro citando a espécie, veio %v", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "turdus-rufiventris.yaml")); string(raw) != baseYAML { //nolint:gosec // G304: pasta temporária do teste
		t.Fatal("uma espécie recusada não deveria ser gravada")
	}
}

func TestRun_ArgumentErrors(t *testing.T) {
	dir := contentDir(t)
	for _, args := range [][]string{{"--dir", dir}, {"--dir", dir, "--missing", "--ids", "x"}, {"--dir", dir, "--ids", "nao-existe", "--dry-run"}} {
		if err := run(context.Background(), args, "", &bytes.Buffer{}, nil); err == nil {
			t.Errorf("%v: esperava erro", args)
		}
	}
}
