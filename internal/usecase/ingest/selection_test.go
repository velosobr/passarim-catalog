// Testes da pontuação e da escolha da foto principal e da galeria (funções
// puras, sem fakes).

package ingest

import (
	"math"
	"strings"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// cl monta uma candidata classificada: id, observação, categoria e pontuação.
func cl(id, obs string, cat domain.PhotoCategory, score float64) classified {
	return classified{cand: PhotoCandidate{SourceID: id, ObservationID: obs},
		verdict: PhotoVerdict{PhotoID: id, Category: cat, Confidence: 0.5}, score: score}
}

// idsOf devolve os ids na ordem, para comparar.
func idsOf(cs []classified) string {
	var out []string
	for _, x := range cs {
		out = append(out, x.cand.SourceID)
	}
	return strings.Join(out, ",")
}

func nearly(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// A fórmula da spec: 0,4 tamanho + 0,3 nitidez + 0,2 votos + 0,1 confiança.
func TestPhotoScore(t *testing.T) {
	v := PhotoVerdict{LargestBirdArea: 0.125, Sharpness: 0.5, Confidence: 0.8}
	// tamanho 0,125/0,25 = 0,5; votos log(1+3)/log(1+3) = 1.
	if got := photoScore(v, 3, 3); !nearly(got, 0.4*0.5+0.3*0.5+0.2*1+0.1*0.8) {
		t.Fatalf("pontuação = %v", got)
	}
	// Ave enorme: o tamanho satura em 1. Ninguém com voto: a parte de votos é 0.
	big := PhotoVerdict{LargestBirdArea: 0.9}
	if got := photoScore(big, 0, 0); !nearly(got, 0.4) {
		t.Fatalf("tamanho saturado e sem votos = %v", got)
	}
	// Ave cortada na borda: metade.
	cut := v
	cut.LargestBirdCut = true
	if got := photoScore(cut, 3, 3); !nearly(got, photoScore(v, 3, 3)/2) {
		t.Fatalf("cortada = %v", got)
	}
}

// scoreAll usa o maior número de votos entre as candidatas da espécie.
func TestScoreAll(t *testing.T) {
	cs := []classified{
		{cand: PhotoCandidate{Votes: 0}, verdict: PhotoVerdict{}},
		{cand: PhotoCandidate{Votes: 9}, verdict: PhotoVerdict{}},
	}
	scoreAll(cs)
	if !nearly(cs[0].score, 0) || !nearly(cs[1].score, 0.2) {
		t.Fatalf("pontuações: %v %v", cs[0].score, cs[1].score)
	}
}

// Principal = a ave sozinha de maior pontuação; depois as outras aves
// sozinhas, depois os bandos; descartes ficam de fora.
func TestChoose_OrderAndDiscards(t *testing.T) {
	cs := []classified{
		cl("flock", "1", domain.PhotoFlock, 0.99),
		cl("nest", "2", domain.PhotoNest, 0.95),
		cl("s-low", "3", domain.PhotoSingleBird, 0.40),
		cl("s-top", "4", domain.PhotoSingleBird, 0.80),
		cl("unk", "5", domain.PhotoUnspecified, 0.90),
	}
	if got := idsOf(choose(cs, 6)); got != "s-top,s-low,flock" {
		t.Fatalf("galeria: %s", got)
	}
}

// No máximo 6 fotos e no máximo 2 da mesma observação.
func TestChoose_LimitsPerGalleryAndObservation(t *testing.T) {
	var cs []classified
	for i, id := range []string{"a1", "a2", "a3", "b1", "c1", "d1", "e1", "f1"} {
		cs = append(cs, cl(id, id[:1], domain.PhotoSingleBird, 1-float64(i)/10))
	}
	if got := idsOf(choose(cs, 6)); got != "a1,a2,b1,c1,d1,e1" {
		t.Fatalf("galeria: %s (a3 é a 3ª da observação a)", got)
	}
}

// O override passa à frente de tudo, mesmo sendo bando.
func TestChoose_OverrideFirst(t *testing.T) {
	ov := cl("ov", "9", domain.PhotoFlock, 0.1)
	ov.override = true
	cs := []classified{cl("s", "1", domain.PhotoSingleBird, 0.9), ov}
	if got := idsOf(choose(cs, 6)); got != "ov,s" {
		t.Fatalf("galeria: %s", got)
	}
}

// Sem ave sozinha e sem override, a espécie fica sem fotos (uma galeria só
// de bandos não é publicada).
func TestChoose_OnlyFlocksMeansNoPhotos(t *testing.T) {
	cs := []classified{cl("f1", "1", domain.PhotoFlock, 0.9), cl("f2", "2", domain.PhotoFlock, 0.8)}
	if got := choose(cs, 6); len(got) != 0 {
		t.Fatalf("esperava nenhuma foto, veio %s", idsOf(got))
	}
}

// Empate de pontuação: desempata pelo id, para a escolha não mudar de uma
// rodada para outra sem motivo.
func TestChoose_TieBreakIsStable(t *testing.T) {
	cs := []classified{cl("20", "1", domain.PhotoSingleBird, 0.5), cl("10", "2", domain.PhotoSingleBird, 0.5)}
	if got := idsOf(choose(cs, 6)); got != "10,20" {
		t.Fatalf("galeria: %s", got)
	}
}

// As marcas do relatório.
func TestCurationFlags(t *testing.T) {
	single := cl("s", "1", domain.PhotoSingleBird, 0.9)
	single.verdict.Confidence = 0.2
	flock := cl("f", "2", domain.PhotoFlock, 0.9)

	if got := curationFlags([]classified{flock}, nil, false, 0.3); len(got) != 1 || got[0] != domain.FlagNoSingleBird {
		t.Fatalf("só bando: %v", got)
	}
	if got := curationFlags([]classified{single}, []classified{single}, false, 0.3); len(got) != 1 || got[0] != domain.FlagLowConfidence {
		t.Fatalf("confiança 0,2 < 0,3: %v", got)
	}
	if got := curationFlags([]classified{single}, []classified{single}, true, 0.1); len(got) != 1 || got[0] != domain.FlagOverrideFailed {
		t.Fatalf("override falhou: %v", got)
	}
	ov := single
	ov.override = true
	if got := curationFlags([]classified{ov}, []classified{ov}, false, 0.3); len(got) != 0 {
		t.Fatalf("a principal do override não ganha low_confidence: %v", got)
	}
}

// O relatório lista todas as candidatas classificadas.
func TestReportCandidates(t *testing.T) {
	x := cl("7", "70", domain.PhotoNest, 0.25)
	got := reportCandidates([]classified{x})
	want := domain.CuratedCandidate{PhotoID: "7", ObservationID: "70", Category: domain.PhotoNest, Score: 0.25, Confidence: 0.5}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v", got)
	}
}
