// O prompt do kidsgen: as regras fixas (prompt.md, embutido no binário) e a
// mensagem com o texto adulto de cada espécie.

package main

import (
	_ "embed" // o import "em branco" habilita a diretiva //go:embed abaixo
	"fmt"
	"strings"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/kidscontent"
)

// systemPrompt são as regras do conteúdo Kids. //go:embed copia o arquivo
// para dentro do binário na compilação: o kidsgen roda de qualquer pasta, e
// o texto das regras fica num arquivo fácil de revisar no PR.
//
//go:embed prompt.md
var systemPrompt string

// userMessage monta a mensagem com o texto adulto de uma espécie: só o nome
// popular, a descrição, os fatos numerados a partir de 0 e os biomas. URLs,
// UFs e o nome científico ficam de fora, para o modelo não ter de onde tirar
// nada além do texto curado.
func userMessage(s domain.Species) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Ave: %s\n\nDescrição adulta:\n%s\n\nFatos adultos (o número é o source_fact):\n",
		s.CommonNamePt, kidscontent.Collapse(s.Description))
	for i, f := range s.Facts {
		fmt.Fprintf(&b, "%d. %s\n", i, kidscontent.Collapse(f.Text))
	}
	fmt.Fprintf(&b, "\nBiomas da ave (primary_biome sai desta lista): %s\n", strings.Join(biomeNames(s), ", "))
	return b.String()
}

// biomeNames devolve os biomas da espécie como texto, na ordem do YAML.
func biomeNames(s domain.Species) []string {
	out := make([]string, len(s.Biomes))
	for i, b := range s.Biomes {
		out[i] = string(b)
	}
	return out
}
