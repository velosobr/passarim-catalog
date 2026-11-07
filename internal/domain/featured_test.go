// Testes do rodízio da "Ave do dia", só com listas em memória.

package domain_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// dayAfter devolve o dia n dias depois de 2026-11-01, no formato AAAA-MM-DD.
func dayAfter(n int) string {
	return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n).Format(time.DateOnly)
}

// simulate roda o rodízio por n dias seguidos, gravando cada escolha como o
// caso de uso faz, e devolve as linhas.
func simulate(t *testing.T, eligible []string, n int) []domain.FeaturedDay {
	t.Helper()
	var rows []domain.FeaturedDay
	for i := range n {
		day := dayAfter(i)
		id, cycle, ok := domain.PickFeatured(day, eligible, rows)
		if !ok {
			t.Fatalf("dia %s sem escolha", day)
		}
		rows = append(rows, domain.FeaturedDay{Day: day, SpeciesID: id, Cycle: cycle})
	}
	return rows
}

// Em cada ciclo, todas as elegíveis aparecem uma vez, sem repetir.
func TestPickFeatured_CycleWithoutRepetition(t *testing.T) {
	eligible := []string{"a", "b", "c", "d", "e"}
	rows := simulate(t, eligible, 15)
	for c := range 3 {
		seen := map[string]bool{}
		for _, r := range rows[c*5 : c*5+5] {
			if r.Cycle != c+1 || seen[r.SpeciesID] {
				t.Fatalf("ciclo %d com repetição ou número errado: %+v", c+1, rows[c*5:c*5+5])
			}
			seen[r.SpeciesID] = true
		}
	}
}

// Na virada de ciclo, a ave de ontem não volta no primeiro dia do ciclo novo.
func TestPickFeatured_NewCycleSkipsYesterday(t *testing.T) {
	for _, eligible := range [][]string{{"a", "b"}, {"a", "b", "c"}, {"a", "b", "c", "d", "e", "f", "g"}} {
		rows := simulate(t, eligible, 10*len(eligible))
		for i := 1; i < len(rows); i++ {
			if rows[i].SpeciesID == rows[i-1].SpeciesID {
				t.Fatalf("%v: a mesma ave em %s e %s", eligible, rows[i-1].Day, rows[i].Day)
			}
		}
	}
}

// Com uma elegível só, ela se repete (não há outra escolha).
func TestPickFeatured_SingleEligibleRepeats(t *testing.T) {
	rows := simulate(t, []string{"a"}, 3)
	if rows[2].SpeciesID != "a" || rows[2].Cycle != 3 {
		t.Fatalf("linhas: %+v", rows)
	}
}

// Uma ave nova entra no ciclo em andamento; a que saiu deixa de ser escolhida.
func TestPickFeatured_EligibleSetChanges(t *testing.T) {
	rows := simulate(t, []string{"a", "b", "c"}, 1)
	used := rows[0].SpeciesID
	var others []string
	for _, id := range []string{"a", "b", "c"} {
		if id != used {
			others = append(others, id)
		}
	}
	// Sai uma das que faltavam, entra "z": o ciclo 1 termina com a outra e "z".
	eligible := []string{others[0], "z"}
	if used != "z" {
		eligible = append(eligible, used)
	}
	for i := 1; i <= 2; i++ {
		id, cycle, _ := domain.PickFeatured(dayAfter(i), eligible, rows)
		if cycle != 1 || id == used || id == others[1] {
			t.Fatalf("dia %d: %s no ciclo %d (usada=%s, removida=%s)", i, id, cycle, used, others[1])
		}
		rows = append(rows, domain.FeaturedDay{Day: dayAfter(i), SpeciesID: id, Cycle: cycle})
	}
	if rows[1].SpeciesID == rows[2].SpeciesID {
		t.Fatalf("repetiu dentro do ciclo: %+v", rows)
	}
}

// Amanhã calculado antes de hoje: hoje não repete a ave de amanhã.
func TestPickFeatured_TomorrowBeforeToday(t *testing.T) {
	eligible := []string{"a", "b", "c", "d"}
	rows := simulate(t, eligible, 1)
	tomorrowID, cycle, _ := domain.PickFeatured(dayAfter(2), eligible, rows)
	rows = append(rows, domain.FeaturedDay{Day: dayAfter(2), SpeciesID: tomorrowID, Cycle: cycle})
	todayID, _, _ := domain.PickFeatured(dayAfter(1), eligible, rows)
	if todayID == tomorrowID || todayID == rows[0].SpeciesID {
		t.Fatalf("hoje=%s amanhã=%s ontem=%s", todayID, tomorrowID, rows[0].SpeciesID)
	}
}

// Sem nenhuma elegível, não há escolha.
func TestPickFeatured_NoEligible(t *testing.T) {
	if _, _, ok := domain.PickFeatured(dayAfter(0), nil, nil); ok {
		t.Fatal("esperava ok=false")
	}
}

// A escolha não depende da ordem em que as elegíveis chegam.
func TestPickFeatured_Deterministic(t *testing.T) {
	a, _, _ := domain.PickFeatured("2026-11-05", []string{"c", "a", "b"}, nil)
	b, _, _ := domain.PickFeatured("2026-11-05", []string{"b", "c", "a"}, nil)
	if a != b {
		t.Fatalf("mesma data, escolhas diferentes: %s %s", a, b)
	}
	if fmt.Sprint(a) == "" {
		t.Fatal("escolha vazia")
	}
}

// "Amanhã antes de hoje" bem na virada de ciclo: o ciclo 1 acabou ontem,
// amanhã foi pedido primeiro e abriu o ciclo 2. Hoje também é do ciclo 2 e
// não pode repetir a ave de amanhã nem a de ontem. Vários começos de data,
// porque a escolha depende do hash do dia.
func TestPickFeatured_TomorrowBeforeTodayAtCycleBoundary(t *testing.T) {
	eligible := []string{"a", "b", "c", "d", "e"}
	for offset := range 60 {
		var rows []domain.FeaturedDay
		for i := range len(eligible) { // o ciclo 1 inteiro, até ontem
			day := dayAfter(offset + i)
			id, cycle, _ := domain.PickFeatured(day, eligible, rows)
			rows = append(rows, domain.FeaturedDay{Day: day, SpeciesID: id, Cycle: cycle})
		}
		yesterday := rows[len(rows)-1]
		today, tomorrow := dayAfter(offset+len(eligible)), dayAfter(offset+len(eligible)+1)
		tomorrowID, cycle, _ := domain.PickFeatured(tomorrow, eligible, rows)
		rows = append(rows, domain.FeaturedDay{Day: tomorrow, SpeciesID: tomorrowID, Cycle: cycle})
		todayID, todayCycle, _ := domain.PickFeatured(today, eligible, rows)
		if todayID == tomorrowID || todayID == yesterday.SpeciesID || todayCycle != 2 {
			t.Fatalf("começo %d: ontem=%s hoje=%s (ciclo %d) amanhã=%s", offset, yesterday.SpeciesID, todayID, todayCycle, tomorrowID)
		}
	}
}
