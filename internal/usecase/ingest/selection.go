// Pontuação e escolha das fotos de uma espécie, depois que o curador disse
// o que cada candidata mostra. São funções puras (sem rede, banco ou
// storage): o IngestPhotos as chama, e os testes as exercitam direto.

package ingest

import (
	"cmp"
	"math"
	"slices"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// maxPerObservation: no máximo 2 fotos da mesma observação na galeria, para
// não mostrar três fotos quase iguais da mesma ave.
const maxPerObservation = 2

// classified é uma candidata que já passou pela licença, foi baixada, ganhou
// a medium e voltou do curador. Guarda o original para gerar a thumb e a
// large se for escolhida. override marca a principal escolhida no YAML.
type classified struct {
	cand     PhotoCandidate
	license  domain.License
	original []byte
	medium   []byte
	verdict  PhotoVerdict
	score    float64
	override bool
}

// photoScore dá uma nota de 0 a 1 a uma foto (pesos da spec, seção 5.3):
//   - 0,4 pelo tamanho da ave: 25% da foto ou mais já vale a nota cheia;
//   - 0,3 pela nitidez;
//   - 0,2 pelos votos, em escala logarítmica (log1p(x) = log(1+x)), relativa
//     à candidata mais votada da espécie: 2 votos contra 200 ainda contam algo;
//   - 0,1 pela confiança do curador.
//
// Ave cortada na borda vale a metade.
func photoScore(v PhotoVerdict, votes, maxVotes int) float64 {
	size := math.Min(v.LargestBirdArea/0.25, 1)
	var voteShare float64
	if maxVotes > 0 {
		voteShare = math.Log1p(float64(votes)) / math.Log1p(float64(maxVotes))
	}
	s := 0.4*size + 0.3*v.Sharpness + 0.2*voteShare + 0.1*v.Confidence
	if v.LargestBirdCut {
		s *= 0.5
	}
	return s
}

// scoreAll calcula a pontuação de cada candidata, com o maior número de
// votos entre todas elas.
func scoreAll(cs []classified) {
	maxVotes := 0
	for _, x := range cs {
		maxVotes = max(maxVotes, x.cand.Votes)
	}
	for i := range cs {
		cs[i].score = photoScore(cs[i].verdict, cs[i].cand.Votes, maxVotes)
	}
}

// choose monta a galeria, com a principal na posição 0:
//  1. a principal do override, se houver (seja qual for a categoria);
//  2. as aves sozinhas, da maior pontuação para a menor;
//  3. os bandos, também por pontuação.
//
// Descartes (ninho, ovos, pena, ave morta, sem ave, outro e unspecified)
// não entram. Até `max` fotos e até 2 por observação. Sem override e sem
// nenhuma ave sozinha, devolve nada: a galeria só de bandos não é publicada,
// porque o contrato diz que a primeira foto é a principal.
func choose(cs []classified, max int) []classified {
	var out []classified
	perObservation := map[string]int{}
	take := func(x classified) {
		if len(out) < max && perObservation[x.cand.ObservationID] < maxPerObservation {
			perObservation[x.cand.ObservationID]++
			out = append(out, x)
		}
	}
	for _, x := range cs {
		if x.override {
			take(x)
		}
	}
	rank := map[domain.PhotoCategory]int{domain.PhotoSingleBird: 0, domain.PhotoFlock: 1}
	var eligible []classified
	for _, x := range cs {
		if _, ok := rank[x.verdict.Category]; ok && !x.override {
			eligible = append(eligible, x)
		}
	}
	// Ordena por categoria, pontuação (maior primeiro) e, no empate, id.
	// cmp.Or devolve o primeiro critério que não empatou.
	slices.SortFunc(eligible, func(a, b classified) int {
		return cmp.Or(
			cmp.Compare(rank[a.verdict.Category], rank[b.verdict.Category]),
			cmp.Compare(b.score, a.score),
			cmp.Compare(a.cand.SourceID, b.cand.SourceID),
		)
	})
	if len(out) == 0 && (len(eligible) == 0 || eligible[0].verdict.Category != domain.PhotoSingleBird) {
		return nil
	}
	for _, x := range eligible {
		take(x)
	}
	return out
}

// curationFlags diz o que o dono precisa conferir nesta espécie:
//   - no_single_bird: nenhuma candidata saiu ave sozinha;
//   - override_failed: a principal do YAML não pôde ser usada;
//   - low_confidence: a principal escolhida pelo MODELO tem confiança abaixo
//     do limite. A do override não é marcada: o dono já olhou essa foto.
//
// A lista nunca é nil: a coluna flags é NOT NULL.
func curationFlags(cs, chosen []classified, overrideFailed bool, lowConfidence float64) []domain.CurationFlag {
	flags := []domain.CurationFlag{}
	if !slices.ContainsFunc(cs, func(x classified) bool { return x.verdict.Category == domain.PhotoSingleBird }) {
		flags = append(flags, domain.FlagNoSingleBird)
	}
	if overrideFailed {
		flags = append(flags, domain.FlagOverrideFailed)
	}
	if len(chosen) > 0 && !chosen[0].override && chosen[0].verdict.Confidence < lowConfidence {
		flags = append(flags, domain.FlagLowConfidence)
	}
	return flags
}

// reportCandidates resume as candidatas para o relatório.
func reportCandidates(cs []classified) []domain.CuratedCandidate {
	out := make([]domain.CuratedCandidate, len(cs))
	for i, x := range cs {
		out[i] = domain.CuratedCandidate{PhotoID: x.cand.SourceID, ObservationID: x.cand.ObservationID,
			Category: x.verdict.Category, Score: x.score, Confidence: x.verdict.Confidence}
	}
	return out
}
