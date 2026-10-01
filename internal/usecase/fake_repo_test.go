package usecase_test

import (
	"context"
	"sort"
	"strings"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

// fakeRepo imita o banco em memória. Testar casos de uso com um "fake"
// (e não com o PostgreSQL real) deixa os testes rápidos e focados na regra.
type fakeRepo struct {
	species   []domain.Species
	lastQuery usecase.ListFilter
	err       error
	// sortNameOverride simula um sort_name GRAVADO no banco que diverge do
	// que domain.NormalizeForSearch(CommonNamePt) recalcularia agora —
	// prova que o cursor vem do valor devolvido pelo repo, não de um
	// recálculo (Review Focus #2). Chave = ID da espécie.
	sortNameOverride map[string]string
}

func (f *fakeRepo) ListSpecies(_ context.Context, q usecase.ListFilter) ([]domain.SpeciesSummary, error) {
	f.lastQuery = q
	if f.err != nil {
		return nil, f.err
	}
	all := append([]domain.Species(nil), f.species...)
	sort.Slice(all, func(i, j int) bool {
		a, b := domain.NormalizeForSearch(all[i].CommonNamePt), domain.NormalizeForSearch(all[j].CommonNamePt)
		if a != b {
			return a < b
		}
		return all[i].ID < all[j].ID
	})
	var out []domain.SpeciesSummary
	for _, s := range all {
		key := domain.NormalizeForSearch(s.CommonNamePt)
		if q.After != nil && (key < q.After.SortName || (key == q.After.SortName && s.ID <= q.After.ID)) {
			continue
		}
		if q.Query != "" && !strings.Contains(key, q.Query) {
			continue
		}
		sortName := key
		if override, ok := f.sortNameOverride[s.ID]; ok {
			sortName = override
		}
		out = append(out, domain.SpeciesSummary{ID: s.ID, CommonNamePt: s.CommonNamePt, ScientificName: s.ScientificName, SortName: sortName})
		if len(out) == q.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeRepo) GetSpecies(_ context.Context, id string) (domain.Species, error) {
	for _, s := range f.species {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.Species{}, domain.ErrNotFound
}

func (f *fakeRepo) CountByBiome(context.Context) (map[domain.Biome]int, error) {
	return map[domain.Biome]int{domain.BiomePantanal: 2, domain.BiomeAmazonia: 1}, f.err
}

func (f *fakeRepo) CountByState(context.Context) (map[string]int, error) {
	return map[string]int{"SP": 3, "AM": 1}, f.err
}

// birds cria n espécies "Ave 01", "Ave 02"...
func birds(n int) []domain.Species {
	out := make([]domain.Species, n)
	for i := range out {
		name := "Ave " + string(rune('A'+i/26)) + string(rune('a'+i%26))
		out[i] = domain.Species{ID: domain.SpeciesID("Genus s" + name[4:]), CommonNamePt: name, ScientificName: "Genus s" + name[4:]}
	}
	return out
}
