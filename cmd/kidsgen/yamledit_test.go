// Testes da edição de texto do YAML: o arquivo editado continua lido pelo
// mesmo loader do seed, com os valores novos, e o resto do arquivo não muda.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/adapter/curated"
	"github.com/velosobr/passarim-catalog/internal/domain"
)

// baseYAML é um arquivo de espécie como os do catálogo, sem as chaves novas.
const baseYAML = `scientific_name: Turdus rufiventris
common_name_pt: Sabiá-laranjeira
family: Turdidae
conservation_status: LC
biomes: [mata_atlantica, cerrado]
states: [SP]
# comentário que precisa continuar no arquivo
description: >
  Ave de peito alaranjado, comum em quintais.
description_source_url: https://pt.wikipedia.org/wiki/Sabi%C3%A1-laranjeira
facts:
  - text: Foi declarada ave-símbolo em 2002.
    source: https://a
  - text: Imita outras aves.
    source: https://b
`

// sampleKids tem textos com ": ", " #" e aspas, que exigem aspas no YAML.
func sampleKids() domain.KidsContent {
	return domain.KidsContent{
		Description: "O sabiá tem a barriga cor de laranja e vive em quintais, parques e matas de quase todo o Brasil. Ele canta cedo.",
		Facts: []domain.KidsFact{{Text: "Em 2002: virou a ave do Brasil.", SourceFact: 0},
			{Text: `Ele imita o "bem-te-vi" #sério.`, SourceFact: 1}},
		DidYouKnow: "O sabiá canta bem cedinho.", GeneratedBy: "claude-opus-5-5", SourceHash: "sha256:abc",
	}
}

// load grava o texto num diretório temporário e o lê com o loader do seed.
func load(t *testing.T, text []byte) domain.Species {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "turdus-rufiventris.yaml"), text, 0o600); err != nil {
		t.Fatal(err)
	}
	species, err := curated.LoadDir(dir)
	if err != nil {
		t.Fatalf("o loader recusou o arquivo editado: %v\n%s", err, text)
	}
	return species[0]
}

func TestApplyKids_WithoutBlock(t *testing.T) {
	out, err := applyKids([]byte(baseYAML), domain.BiomeMataAtlantica, sampleKids())
	if err != nil {
		t.Fatal(err)
	}
	s := load(t, out)
	want := sampleKids()
	if s.PrimaryBiome != domain.BiomeMataAtlantica || s.Kids.Description != want.Description ||
		s.Kids.Facts[0].Text != want.Facts[0].Text || s.Kids.Facts[1].Text != want.Facts[1].Text || s.Kids.DidYouKnow != want.DidYouKnow {
		t.Fatalf("lido de volta: bioma=%q kids=%+v", s.PrimaryBiome, s.Kids)
	}
	text := string(out)
	if !strings.Contains(text, "biomes: [mata_atlantica, cerrado]\nprimary_biome: mata_atlantica\nstates:") {
		t.Fatalf("primary_biome deveria vir logo depois de biomes:\n%s", text)
	}
	if !strings.HasPrefix(text, baseYAML[:strings.Index(baseYAML, "biomes:")]) || !strings.Contains(text, "# comentário que precisa continuar") {
		t.Fatalf("o resto do arquivo mudou:\n%s", text)
	}
	if !strings.HasSuffix(text, "  source_hash: sha256:abc\n") {
		t.Fatalf("o bloco kids: deveria ir para o fim:\n%s", text)
	}
}

// Rodar de novo troca a linha e o bloco, sem duplicar nada, e uma chave
// depois do bloco continua lá.
func TestApplyKids_ReplacesExisting(t *testing.T) {
	first, err := applyKids([]byte(baseYAML), domain.BiomeMataAtlantica, sampleKids())
	if err != nil {
		t.Fatal(err)
	}
	withKeyAfter := string(first) + "size_cm: 23\n"
	k := sampleKids()
	k.DidYouKnow = "Ele mora no cerrado também."
	out, err := applyKids([]byte(withKeyAfter), domain.BiomeCerrado, k)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if strings.Count(text, "primary_biome:") != 1 || strings.Count(text, "kids:") != 1 || !strings.HasSuffix(text, "size_cm: 23\n") {
		t.Fatalf("troca errada:\n%s", text)
	}
	s := load(t, out)
	if s.PrimaryBiome != domain.BiomeCerrado || s.Kids.DidYouKnow != k.DidYouKnow || s.SizeCm == nil || *s.SizeCm != 23 {
		t.Fatalf("lido de volta: bioma=%q kids=%+v size=%v", s.PrimaryBiome, s.Kids, s.SizeCm)
	}
}

// Um arquivo que termina sem quebra de linha ganha a quebra antes do bloco.
func TestApplyKids_NoTrailingNewline(t *testing.T) {
	out, err := applyKids([]byte(strings.TrimSuffix(baseYAML, "\n")), domain.BiomeMataAtlantica, sampleKids())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "source: https://b\nkids:\n") {
		t.Fatalf("faltou a quebra antes do bloco:\n%s", out)
	}
	load(t, out)
}

// biomes: em várias linhas é recusado com uma mensagem clara.
func TestApplyKids_BiomesNotInline(t *testing.T) {
	multi := strings.Replace(baseYAML, "biomes: [mata_atlantica, cerrado]", "biomes:\n  - mata_atlantica\n  - cerrado", 1)
	if _, err := applyKids([]byte(multi), domain.BiomeMataAtlantica, sampleKids()); err == nil || !strings.Contains(err.Error(), "numa linha só") {
		t.Fatalf("esperava o erro do formato de biomes:, veio %v", err)
	}
}
