// Testes do validador do bloco kids:, um caso por regra da spec (seção 3.2).

package kidscontent_test

import (
	"github.com/velosobr/passarim-catalog/internal/domain"
)

// adult devolve a espécie só com o texto adulto (sem Kids). Os espaços
// sobrando na descrição são de propósito: o carimbo os colapsa.
func adult() domain.Species {
	return domain.Species{
		ID: "turdus-rufiventris", CommonNamePt: "Sabiá-laranjeira",
		Description: "O sabiá  canta por dois minutos.\n Ele vive em quintais.",
		Facts: []domain.Fact{{Text: "Foi declarado ave nacional em 2002.", Source: "https://a"},
			{Text: "Imita outras aves.", Source: "https://b"}},
		Biomes: []domain.Biome{domain.BiomeMataAtlantica, domain.BiomeCerrado},
		States: []string{"SP"},
	}
}
