// "Ave do dia" (frente B): a regra pura do rodízio. A mesma ave aparece nas
// duas jornadas e o dia inteiro; dentro de um ciclo, nenhuma ave se repete
// antes de todas as elegíveis aparecerem uma vez. O caso de uso grava cada
// escolha na tabela featured_day; aqui só a conta, testada com listas.

package domain

import (
	"hash/fnv"
	"slices"
)

// FeaturedDay é uma linha da tabela featured_day. Day está no formato
// AAAA-MM-DD: nesse formato, comparar os textos é comparar as datas.
type FeaturedDay struct {
	Day       string
	SpeciesID string
	Cycle     int
}

// PickFeatured escolhe a ave do dia `day`.
//
//   - eligible são os ids das espécies que podem ser escolhidas hoje.
//   - cycleRows são as linhas do ciclo da linha mais recente antes de `day`
//     (o repositório devolve só esse ciclo; pode haver dias depois de `day`,
//     se amanhã foi pedido antes de hoje).
//
// A regra:
//  1. prev = a linha mais recente com Day < day. Sem nenhuma, é o ciclo 1.
//  2. As "usadas" são as aves das linhas do ciclo de prev com Day <> day.
//     Contar também os dias depois de `day` evita repetir a ave de amanhã.
//  3. Restantes = elegíveis menos usadas, em ordem alfabética.
//  4. Sem restantes, começa um ciclo novo com todas as elegíveis, menos a
//     ave de prev se houver mais de uma elegível (senão a última ave de um
//     ciclo voltaria no primeiro dia do seguinte).
//  5. A escolhida é restantes[hash(day) % len(restantes)].
//
// ok=false quando não há nenhuma elegível.
func PickFeatured(day string, eligible []string, cycleRows []FeaturedDay) (speciesID string, cycle int, ok bool) {
	if len(eligible) == 0 {
		return "", 0, false
	}
	sorted := slices.Clone(eligible)
	slices.Sort(sorted)

	var prev *FeaturedDay
	for i := range cycleRows {
		r := &cycleRows[i]
		if r.Day < day && (prev == nil || r.Day > prev.Day) {
			prev = r
		}
	}
	cycle = 1
	used := map[string]bool{}
	if prev != nil {
		cycle = prev.Cycle
		for _, r := range cycleRows {
			if r.Cycle == prev.Cycle && r.Day != day {
				used[r.SpeciesID] = true
			}
		}
	}

	remaining := without(sorted, func(id string) bool { return used[id] })
	if len(remaining) == 0 {
		cycle++
		remaining = sorted
		if prev != nil && len(sorted) > 1 {
			remaining = without(sorted, func(id string) bool { return id == prev.SpeciesID })
		}
	}
	return remaining[dayHash(day)%uint64(len(remaining))], cycle, true
}

// without devolve os ids para os quais skip é falso, na mesma ordem.
func without(ids []string, skip func(string) bool) []string {
	var out []string
	for _, id := range ids {
		if !skip(id) {
			out = append(out, id)
		}
	}
	return out
}

// dayHash é o FNV-1a de 64 bits do texto da data. FNV é uma função de hash
// simples e rápida: o mesmo texto dá sempre o mesmo número, e datas
// vizinhas dão números bem diferentes, o que espalha as escolhas.
func dayHash(day string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(day)) // Write de um hash nunca falha
	return h.Sum64()
}
