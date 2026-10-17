// Caso de uso do canto (xeno-canto): escolher a melhor gravação aceitável,
// converter para AAC e trocar o canto da espécie no banco.

package ingest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// IngestAudio escolhe a melhor gravação do canto de uma espécie.
type IngestAudio struct {
	Source      AudioSource
	Downloader  Downloader
	Audio       AudioProcessor
	Store       MediaStore
	Repo        MediaRepository
	Policy      domain.LicensePolicy
	MaxDuration time.Duration
}

// minGoodDurationMs: gravações com menos de 5 s são preteridas, porque mal
// dá para ouvir o canto.
const minGoodDurationMs = 5000

// acceptedAudio é uma gravação que passou na licença e na qualidade.
type acceptedAudio struct {
	cand    AudioCandidate
	license domain.License
}

// qualityRank: A é a melhor. D e E (e desconhecidas) ficam de fora.
func qualityRank(q string) int {
	switch strings.ToUpper(q) {
	case "A":
		return 0
	case "B":
		return 1
	case "C":
		return 2
	}
	return -1
}

// Run trata um job de canto: filtra por licença e qualidade, ordena da
// melhor para a pior e grava a primeira que der certo. sort.SliceStable
// mantém a ordem original entre empates.
func (u IngestAudio) Run(ctx context.Context, job Job) error {
	cands, err := u.Source.FindRecordings(ctx, job.ScientificName)
	if err != nil {
		return fmt.Errorf("buscar gravações: %w", err)
	}
	var accepted []acceptedAudio
	for _, c := range cands {
		lic, err := domain.ParseLicense(c.License)
		if err != nil || !u.Policy.Accepts(lic) || qualityRank(c.Quality) < 0 || !validSourceID(c.SourceID) {
			continue
		}
		accepted = append(accepted, acceptedAudio{c, lic})
	}
	if len(accepted) == 0 {
		// Ave sem canto aceitável: não é erro, e só o canto é removido (fotos ficam).
		if err := u.Repo.ReplaceAudio(ctx, job.SpeciesID, nil); err != nil {
			return err
		}
		removeOrphans(ctx, u.Store, audioPrefix(job.SpeciesID), nil)
		return nil
	}

	// Melhor primeiro: qualidade; depois "song"; depois duração >= 5 s; depois a mais curta.
	sort.SliceStable(accepted, func(i, j int) bool {
		a, b := accepted[i].cand, accepted[j].cand
		if qa, qb := qualityRank(a.Quality), qualityRank(b.Quality); qa != qb {
			return qa < qb
		}
		if sa, sb := isSong(a), isSong(b); sa != sb {
			return sa
		}
		if ga, gb := a.DurationMs >= minGoodDurationMs, b.DurationMs >= minGoodDurationMs; ga != gb {
			return ga
		}
		return a.DurationMs < b.DurationMs
	})

	var lastErr error
	for _, a := range accepted {
		audio, err := u.processOne(ctx, job, a)
		if err != nil {
			lastErr = err // tenta a próxima candidata
			continue
		}
		if err := u.Repo.ReplaceAudio(ctx, job.SpeciesID, audio); err != nil {
			return err
		}
		// Só depois de o banco apontar para o canto novo apagamos o antigo.
		removeOrphans(ctx, u.Store, audioPrefix(job.SpeciesID), map[string]bool{audio.Key: true})
		return nil
	}
	return fmt.Errorf("nenhuma gravação processada: %w", lastErr)
}

// isSong diz se a gravação é um canto ("song"), e não um chamado ("call").
func isSong(c AudioCandidate) bool { return strings.Contains(strings.ToLower(c.Type), "song") }

// processOne baixa, converte para AAC (cortando em MaxDuration) e grava. A
// duração gravada é limitada ao corte.
func (u IngestAudio) processOne(ctx context.Context, job Job, a acceptedAudio) (*domain.Audio, error) {
	data, _, err := u.Downloader.Fetch(ctx, a.cand.URL, DownloadAudio)
	if err != nil {
		return nil, err
	}
	aac, err := u.Audio.ToAAC(ctx, data, u.MaxDuration)
	if err != nil {
		return nil, err
	}
	key := audioKey(job.SpeciesID, a.cand.SourceID)
	if err := u.Store.Put(ctx, key, "audio/aac", aac); err != nil {
		return nil, fmt.Errorf("gravar no storage: %w", err)
	}
	durationMs := a.cand.DurationMs
	if max := int(u.MaxDuration.Milliseconds()); durationMs > max {
		durationMs = max
	}
	return &domain.Audio{Key: key, DurationMs: durationMs,
		Credit: domain.Credit{Author: a.cand.Author, License: string(a.license), Source: "xeno-canto", SourceURL: a.cand.PageURL}}, nil
}
