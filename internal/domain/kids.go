// Conteúdo Kids de uma espécie: o texto para crianças de 8 a 12 anos, gerado
// por IA a partir do texto adulto curado e revisado por uma pessoa no PR do
// YAML (frente B). Este arquivo tem os tipos e a validação da ESTRUTURA do
// bloco; as regras do texto (frases de até 15 palavras, números, hash do
// texto adulto) ficam no validador do conteúdo (pacote kidscontent).

package domain

import (
	"fmt"
	"slices"
	"strings"
)

// KidsFact é um fato para crianças. SourceFact é a posição, a partir de 0,
// do fato adulto de onde ele saiu: o Kids nunca traz um fato novo.
type KidsFact struct {
	Text       string
	SourceFact int
}

// KidsContent é o bloco kids: do YAML. GeneratedBy (o id do modelo) e
// SourceHash (o hash do texto adulto de origem) servem à revisão e ao lint;
// o banco guarda só o que o app mostra.
type KidsContent struct {
	Description string
	Facts       []KidsFact
	DidYouKnow  string
	GeneratedBy string
	SourceHash  string
}

// validateKids confere o bioma principal e a estrutura do bloco Kids:
//   - o bioma principal, quando existe, é um dos biomas da espécie (no Kids
//     ele é o "mundo" da ave, e uma ave não pode morar num mundo onde não vive);
//   - com bloco Kids, o bioma principal é obrigatório (os dois nascem juntos
//     no kidsgen) e todos os campos do bloco também;
//   - cada fato Kids aponta para um fato adulto que existe.
func (s Species) validateKids() error {
	if s.PrimaryBiome != "" {
		if _, err := ParseBiome(string(s.PrimaryBiome)); err != nil {
			return invalid("primary_biome", err.Error())
		}
		if !slices.Contains(s.Biomes, s.PrimaryBiome) {
			return invalid("primary_biome", fmt.Sprintf("%q não está entre os biomas da espécie", s.PrimaryBiome))
		}
	}
	k := s.Kids
	if k == nil {
		return nil
	}
	if s.PrimaryBiome == "" {
		return invalid("primary_biome", "obrigatório quando há o bloco kids")
	}
	if strings.TrimSpace(k.Description) == "" || strings.TrimSpace(k.DidYouKnow) == "" {
		return invalid("kids", "description e did_you_know são obrigatórios")
	}
	if strings.TrimSpace(k.GeneratedBy) == "" || strings.TrimSpace(k.SourceHash) == "" {
		return invalid("kids", "generated_by e source_hash são obrigatórios")
	}
	if len(k.Facts) == 0 {
		return invalid("kids.facts", "pelo menos um fato")
	}
	for i, f := range k.Facts {
		if strings.TrimSpace(f.Text) == "" {
			return invalid("kids.facts", fmt.Sprintf("fato %d sem texto", i+1))
		}
		if f.SourceFact < 0 || f.SourceFact >= len(s.Facts) {
			return invalid("kids.facts", fmt.Sprintf("fato %d aponta para o fato adulto %d, que não existe", i+1, f.SourceFact))
		}
	}
	return nil
}
