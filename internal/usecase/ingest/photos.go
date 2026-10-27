// Caso de uso das fotos (iNaturalist) com a curadoria da frente A: buscar
// candidatas, aplicar licença e override, baixar, gerar a medium, perguntar
// ao curador o que cada foto mostra, escolher a principal e a galeria, gerar
// thumb e large só das escolhidas e trocar tudo no banco com o relatório.

package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// IngestPhotos escolhe e grava as fotos de uma espécie.
//
//   - MaxPhotos: tamanho da galeria (6); a posição 0 é a principal.
//   - MaxCandidates: quantas candidatas vão ao curador (40).
//   - Observations: quantas observações a busca pede (20, com até 3 fotos cada).
//   - LowConfidence: abaixo disso, a principal do modelo é marcada no relatório.
//   - Timeout: prazo do job inteiro (PHOTO_JOB_TIMEOUT).
type IngestPhotos struct {
	Source        PhotoSource
	Downloader    Downloader
	Images        ImageProcessor
	Curator       PhotoCurator
	Store         MediaStore
	Repo          MediaRepository
	Overrides     OverrideSource
	Policy        domain.LicensePolicy
	MaxPhotos     int
	MaxCandidates int
	Observations  int
	LowConfidence float64
	Timeout       time.Duration
}

// acceptedPhoto é uma candidata que passou na licença, com a licença já
// interpretada. override marca a principal do YAML.
type acceptedPhoto struct {
	cand     PhotoCandidate
	license  domain.License
	override bool
}

// Run trata um job de fotos, na ordem da seção 5.2 da spec. Se o curador
// está fora do ar, devolve ErrCuratorUnavailable antes de tocar no storage
// ou no banco: as fotos atuais ficam, e o Runner adia o job.
func (u IngestPhotos) Run(ctx context.Context, job Job) error {
	if u.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, u.Timeout)
		defer cancel()
	}
	override, err := u.Overrides.PhotoOverride(ctx, job.SpeciesID)
	if err != nil {
		return err
	}
	found, err := u.Source.FindPhotos(ctx, job.ScientificName, u.Observations)
	if err != nil {
		return fmt.Errorf("buscar fotos: %w", err)
	}

	// Passos 1 a 3: licença, exclusões, a principal do override na frente e o corte.
	accepted := u.accept(found, override)
	overrideFailed := false
	if override.HasPrimary() {
		primary, ok, err := u.findPrimary(ctx, override)
		if err != nil {
			return err
		}
		if ok {
			accepted = withPrimaryFirst(accepted, primary)
		} else {
			overrideFailed = true
		}
	}
	accepted = accepted[:min(len(accepted), u.MaxCandidates)]
	if len(accepted) == 0 {
		// A fonte respondeu e não há nada utilizável: limpar é o resultado correto.
		return u.publish(ctx, job, nil, domain.CurationReport{Flags: curationFlags(nil, nil, overrideFailed, u.LowConfidence)})
	}

	// Passo 4: baixar e gerar a medium. Uma foto ruim não derruba as outras.
	prepared, lastErr := u.prepare(ctx, accepted)
	if len(prepared) == 0 {
		// Havia candidatas boas e todas falharam: provavelmente algo temporário.
		// Devolvemos erro (o job tenta de novo) em vez de apagar fotos que já temos.
		return fmt.Errorf("nenhuma foto processada: %w", lastErr)
	}
	if accepted[0].override && !prepared[0].override {
		overrideFailed = true // a principal do YAML não baixou ou não converteu
	}

	// Passo 5: o curador, em lotes de 8.
	modelVersion, err := u.classify(ctx, prepared)
	if err != nil {
		return err
	}
	if prepared[0].override && prepared[0].verdict.Category == domain.PhotoUnspecified {
		prepared[0].override, overrideFailed = false, true // a principal do YAML não abriu no curador
	}

	// Passo 6: pontuar e escolher.
	scoreAll(prepared)
	chosen := choose(prepared, u.MaxPhotos)

	// Passo 7: thumb e large só das escolhidas, e as três no storage.
	photos, err := u.finish(ctx, job, chosen, modelVersion)
	if err != nil {
		return err
	}
	report := domain.CurationReport{ModelVersion: modelVersion, Candidates: reportCandidates(prepared),
		Flags: curationFlags(prepared, chosen, overrideFailed, u.LowConfidence)}
	if len(photos) > 0 {
		report.PrimaryPhotoID = photos[0].SourcePhotoID
	}
	// Passos 8 e 9: banco (fotos + relatório numa transação) e limpeza.
	return u.publish(ctx, job, photos, report)
}

// accept fica com as candidatas de licença aceita, id seguro e fora das
// exclusões, sem repetir a mesma foto. A licença é conferida ANTES de
// baixar: nem tocamos em arquivos que não podemos usar.
func (u IngestPhotos) accept(cands []PhotoCandidate, override domain.PhotoOverride) []acceptedPhoto {
	var out []acceptedPhoto
	seen := map[string]bool{}
	for _, c := range cands {
		lic, ok := u.license(c)
		if !ok || seen[c.SourceID] || override.Excludes(c.SourceID) {
			continue
		}
		seen[c.SourceID] = true
		out = append(out, acceptedPhoto{cand: c, license: lic})
	}
	return out
}

// license interpreta a licença da candidata e diz se ela e o id servem.
func (u IngestPhotos) license(c PhotoCandidate) (domain.License, bool) {
	lic, err := domain.ParseLicense(c.License)
	return lic, err == nil && u.Policy.Accepts(lic) && validSourceID(c.SourceID)
}

// findPrimary busca a principal do YAML, mesmo fora das candidatas. Ela
// passa pela mesma política de licença. ok=false: sumiu ou a licença não
// serve mais (a espécie é marcada override_failed).
func (u IngestPhotos) findPrimary(ctx context.Context, o domain.PhotoOverride) (acceptedPhoto, bool, error) {
	c, found, err := u.Source.FindObservationPhoto(ctx, o.PrimaryObservationID, o.PrimaryPhotoID)
	if err != nil {
		return acceptedPhoto{}, false, fmt.Errorf("buscar a foto do override: %w", err)
	}
	if !found {
		return acceptedPhoto{}, false, nil
	}
	lic, ok := u.license(c)
	return acceptedPhoto{cand: c, license: lic, override: true}, ok, nil
}

// withPrimaryFirst põe a principal do override na frente e tira a cópia
// dela que tenha vindo nas candidatas.
func withPrimaryFirst(accepted []acceptedPhoto, primary acceptedPhoto) []acceptedPhoto {
	out := []acceptedPhoto{primary}
	for _, a := range accepted {
		if a.cand.SourceID != primary.cand.SourceID {
			out = append(out, a)
		}
	}
	return out
}

// prepare baixa cada candidata e gera a medium. As que falham ficam de fora;
// lastErr guarda o último erro, para a mensagem quando todas falham.
func (u IngestPhotos) prepare(ctx context.Context, accepted []acceptedPhoto) ([]classified, error) {
	var out []classified
	var lastErr error
	for _, a := range accepted {
		data, _, err := u.Downloader.Fetch(ctx, a.cand.URL, DownloadImage)
		if err != nil {
			lastErr = err
			continue
		}
		medium, err := u.Images.Medium(ctx, data)
		if err != nil {
			lastErr = err
			continue
		}
		out = append(out, classified{cand: a.cand, license: a.license, original: data, medium: medium, override: a.override})
	}
	return out, lastErr
}

// classify manda as mediums ao curador em lotes de MaxCuratorBatch e guarda
// o veredito de cada uma. Devolve a versão do modelo.
func (u IngestPhotos) classify(ctx context.Context, cs []classified) (string, error) {
	var version string
	for start := 0; start < len(cs); start += MaxCuratorBatch {
		batch := cs[start:min(start+MaxCuratorBatch, len(cs))]
		in := make([]CuratorPhoto, len(batch))
		for i, x := range batch {
			in[i] = CuratorPhoto{PhotoID: x.cand.SourceID, Image: x.medium}
		}
		verdicts, v, err := u.Curator.ClassifyPhotos(ctx, in)
		if err != nil {
			return "", fmt.Errorf("classificar fotos: %w", err)
		}
		for i := range batch {
			batch[i].verdict = verdicts[i] // batch aponta para o mesmo array de cs
		}
		version = v
	}
	return version, nil
}

// finish gera a thumb e a large das escolhidas e grava as três variantes no
// storage. O crédito guarda a página da observação, para o app linkar o
// original. Uma falha aqui devolve erro: a medium da mesma foto já foi
// gerada, então é quase sempre algo temporário, e o job tenta de novo.
func (u IngestPhotos) finish(ctx context.Context, job Job, chosen []classified, modelVersion string) ([]domain.Photo, error) {
	photos := make([]domain.Photo, 0, len(chosen))
	for _, x := range chosen {
		v, err := u.Images.ThumbAndLarge(ctx, x.original)
		if err != nil {
			return nil, fmt.Errorf("gerar thumb e large da foto %s: %w", x.cand.SourceID, err)
		}
		p := domain.Photo{
			ThumbKey:  photoKey(job.SpeciesID, x.cand.SourceID, "thumb"),
			MediumKey: photoKey(job.SpeciesID, x.cand.SourceID, "medium"),
			LargeKey:  photoKey(job.SpeciesID, x.cand.SourceID, "large"),
			Width:     v.Width, Height: v.Height,
			Credit:        domain.Credit{Author: x.cand.Author, License: string(x.license), Source: "inaturalist", SourceURL: x.cand.PageURL},
			SourcePhotoID: x.cand.SourceID, Category: x.verdict.Category, Score: x.score, ModelVersion: modelVersion,
		}
		for key, body := range map[string][]byte{p.ThumbKey: v.Thumb, p.MediumKey: x.medium, p.LargeKey: v.Large} {
			if err := u.Store.Put(ctx, key, "image/webp", body); err != nil {
				return nil, errors.Join(errors.New("gravar no storage"), err)
			}
		}
		photos = append(photos, p)
	}
	return photos, nil
}

// publish troca as fotos no banco (com o relatório) e, só depois de o banco
// apontar para as novas, apaga do storage os arquivos que ninguém usa mais.
func (u IngestPhotos) publish(ctx context.Context, job Job, photos []domain.Photo, report domain.CurationReport) error {
	if err := u.Repo.ReplacePhotos(ctx, job.SpeciesID, photos, report); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, p := range photos {
		keep[p.ThumbKey], keep[p.MediumKey], keep[p.LargeKey] = true, true, true
	}
	removeOrphans(ctx, u.Store, photoPrefix(job.SpeciesID), keep)
	return nil
}
