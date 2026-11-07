// Testes das regras de texto: frases, palavras, palavras proibidas e o
// carimbo do texto adulto.

package kidscontent_test

import (
	"fmt"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/kidscontent"
)

func TestSentences(t *testing.T) {
	cases := map[string]string{
		"Ele canta. Ele voa!":         "[Ele canta. Ele voa!]",
		"Tem 2.000 penas. E voa.":     "[Tem 2.000 penas. E voa.]",
		"Será? Sim... Voa":            "[Será? Sim... Voa]",
		"  Uma frase só, sem ponto  ": "[Uma frase só, sem ponto]",
	}
	for in, want := range cases {
		got := kidscontent.Sentences(in)
		if fmt.Sprintf("%v", got) != want || (in == "Tem 2.000 penas. E voa." && len(got) != 2) {
			t.Errorf("%q: veio %q (%d frases)", in, got, len(got))
		}
	}
}

func TestWords(t *testing.T) {
	cases := map[string]int{"O tico-tico canta.": 3, "Ele voa — e pousa.": 4, "Tem 2 asas.": 3, "": 0}
	for in, want := range cases {
		if got := kidscontent.Words(in); got != want {
			t.Errorf("%q: %d palavras, esperava %d", in, got, want)
		}
	}
}

func TestForbiddenWord(t *testing.T) {
	cases := map[string]string{"Ele tem SANGUE frio.": "SANGUE", "Viu uma carcaça.": "carcaça", "O sabiá canta.": "", "Ficou ensanguentado.": "ensanguentado"}
	for in, want := range cases {
		if got := kidscontent.ForbiddenWord(in); got != want {
			t.Errorf("%q: veio %q, esperava %q", in, got, want)
		}
	}
}

// O carimbo tem valor fixo para o texto canônico da spec, ignora espaços
// sobrando, a ordem dos biomas, as fontes e as UFs, e muda com a descrição,
// um fato ou um bioma.
func TestSourceHash(t *testing.T) {
	base := adult()
	want := "sha256:6c44eb3f2e5b562ae1620d990cf24ce8672a7c937df78e2eb9160bec389d6eae"
	if got := kidscontent.SourceHash(base); got != want {
		t.Fatalf("carimbo %s, esperava %s", got, want)
	}
	same := adult()
	same.Biomes = []domain.Biome{domain.BiomeCerrado, domain.BiomeMataAtlantica}
	same.Facts[0].Source, same.States = "https://outra", []string{"RJ"}
	same.Description = "  O sabiá canta por dois minutos. Ele vive em quintais. "
	if kidscontent.SourceHash(same) != want {
		t.Fatal("ordem dos biomas, fontes, UFs e espaços não deveriam mudar o carimbo")
	}
	for name, change := range map[string]func(*domain.Species){
		"descrição": func(s *domain.Species) { s.Description += " Mais." },
		"fato":      func(s *domain.Species) { s.Facts[1].Text = "Outro fato." },
		"bioma":     func(s *domain.Species) { s.Biomes = append(s.Biomes, domain.BiomePampa) },
	} {
		s := adult()
		change(&s)
		if kidscontent.SourceHash(s) == want {
			t.Errorf("mudar %s deveria mudar o carimbo", name)
		}
	}
}
