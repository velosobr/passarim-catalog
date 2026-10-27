// Testes do relatório de curadoria em Markdown.

package ingest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// As espécies marcadas vêm antes, cada linha com a principal, as marcas e a
// contagem de candidatas por tipo.
func TestFormatCurationReport(t *testing.T) {
	at := time.Date(2026, 10, 27, 14, 30, 0, 0, time.UTC)
	entries := []domain.CurationReportEntry{
		{SpeciesID: "turdus-rufiventris", CommonName: "Sabiá-laranjeira", RanAt: at, Report: domain.CurationReport{
			ModelVersion: "m@2", PrimaryPhotoID: "11",
			Candidates: []domain.CuratedCandidate{{PhotoID: "11", Category: domain.PhotoSingleBird, Confidence: 0.61}}}},
		{SpeciesID: "crotophaga-ani", CommonName: "Anu-preto", RanAt: at, Report: domain.CurationReport{
			ModelVersion: "m@2",
			Candidates:   []domain.CuratedCandidate{{PhotoID: "21", Category: domain.PhotoFlock}, {PhotoID: "22", Category: domain.PhotoNest}},
			Flags:        []domain.CurationFlag{domain.FlagNoSingleBird}}},
	}
	got := ingest.FormatCurationReport(entries)
	for _, want := range []string{
		"2 espécies, 1 marcada",
		"| Anu-preto (`crotophaga-ani`) | — | — | no_single_bird | 0 aves sozinhas, 1 bando, 1 descarte | 2026-10-27 14:30 |",
		"| Sabiá-laranjeira (`turdus-rufiventris`) | [11](https://www.inaturalist.org/photos/11) | 0.61 |  | 1 ave sozinha, 0 bandos, 0 descartes | 2026-10-27 14:30 |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("faltou %q em:\n%s", want, got)
		}
	}
	if strings.Index(got, "Anu-preto") > strings.Index(got, "Sabiá") {
		t.Error("as marcadas vêm primeiro")
	}
}
