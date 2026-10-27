// O relatório de curadoria em Markdown, que o comando "worker
// curation-report" imprime. O dono o usa para conferir as 41 espécies:
// as marcadas (low_confidence, no_single_bird, override_failed) vêm
// primeiro, e cada principal tem o link da foto no iNaturalist.

package ingest

import (
	"fmt"
	"slices"
	"strings"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// FormatCurationReport monta o relatório: um resumo e uma tabela, com as
// espécies marcadas primeiro e, dentro de cada grupo, a ordem do id.
func FormatCurationReport(entries []domain.CurationReportEntry) string {
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b domain.CurationReportEntry) int {
		af, bf := len(a.Report.Flags) > 0, len(b.Report.Flags) > 0
		if af != bf {
			if af {
				return -1
			}
			return 1
		}
		return strings.Compare(a.SpeciesID, b.SpeciesID)
	})
	flagged := 0
	for _, e := range entries {
		if len(e.Report.Flags) > 0 {
			flagged++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Relatório de curadoria\n\n%d espécies, %d %s.\n\n", len(entries), flagged, plural(flagged, "marcada", "marcadas"))
	b.WriteString("| Espécie | Principal | Confiança | Marcas | Candidatas | Rodou em |\n|---|---|---|---|---|---|\n")
	for _, e := range sorted {
		r := e.Report
		primary, confidence := "—", "—"
		if r.PrimaryPhotoID != "" {
			primary = fmt.Sprintf("[%s](https://www.inaturalist.org/photos/%s)", r.PrimaryPhotoID, r.PrimaryPhotoID)
			for _, c := range r.Candidates {
				if c.PhotoID == r.PrimaryPhotoID {
					confidence = fmt.Sprintf("%.2f", c.Confidence)
				}
			}
		}
		flags := make([]string, len(r.Flags))
		for i, f := range r.Flags {
			flags[i] = string(f)
		}
		fmt.Fprintf(&b, "| %s (`%s`) | %s | %s | %s | %s | %s |\n", e.CommonName, e.SpeciesID, primary, confidence,
			strings.Join(flags, ", "), counts(r.Candidates), e.RanAt.Format("2006-01-02 15:04"))
	}
	return b.String()
}

// counts resume as candidatas: aves sozinhas, bandos e o resto (descartes).
func counts(cs []domain.CuratedCandidate) string {
	var single, flock, other int
	for _, c := range cs {
		switch c.Category {
		case domain.PhotoSingleBird:
			single++
		case domain.PhotoFlock:
			flock++
		default:
			other++
		}
	}
	return fmt.Sprintf("%d %s, %d %s, %d %s", single, plural(single, "ave sozinha", "aves sozinhas"),
		flock, plural(flock, "bando", "bandos"), other, plural(other, "descarte", "descartes"))
}

// plural escolhe a forma da palavra pelo número (1 = singular).
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
