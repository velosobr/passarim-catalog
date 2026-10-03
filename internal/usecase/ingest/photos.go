package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// IngestPhotos busca fotos de uma espécie, processa e grava.
type IngestPhotos struct {
	Source     PhotoSource
	Downloader Downloader
	Images     ImageProcessor
	Store      MediaStore
	Repo       MediaRepository
	Policy     domain.LicensePolicy
	MaxPhotos  int
}

type acceptedPhoto struct {
	cand    PhotoCandidate
	license domain.License
}

func (u IngestPhotos) Run(ctx context.Context, job Job) error {
	// Pedimos o dobro: algumas candidatas serão descartadas (licença) ou falharão.
	cands, err := u.Source.FindPhotos(ctx, job.ScientificName, u.MaxPhotos*2)
	if err != nil {
		return fmt.Errorf("buscar fotos: %w", err)
	}

	// Descartar a licença ANTES de baixar: nem tocamos em arquivos que não podemos usar.
	var accepted []acceptedPhoto
	seen := map[string]bool{}
	for _, c := range cands {
		lic, err := domain.ParseLicense(c.License)
		if err != nil || !u.Policy.Accepts(lic) || !validSourceID(c.SourceID) || seen[c.SourceID] {
			continue
		}
		seen[c.SourceID] = true
		accepted = append(accepted, acceptedPhoto{c, lic})
	}
	if len(accepted) == 0 {
		// A fonte respondeu e não há nada utilizável: limpar é o resultado correto.
		if err := u.Repo.ReplacePhotos(ctx, job.SpeciesID, nil); err != nil {
			return err
		}
		removeOrphans(ctx, u.Store, photoPrefix(job.SpeciesID), nil)
		return nil
	}

	var photos []domain.Photo
	var lastErr error
	for _, a := range accepted {
		if len(photos) == u.MaxPhotos {
			break
		}
		photo, err := u.processOne(ctx, job, a)
		if err != nil {
			lastErr = err // uma foto ruim não derruba as outras
			continue
		}
		photos = append(photos, photo)
	}
	if len(photos) == 0 {
		// Havia candidatas boas e todas falharam: provavelmente algo temporário.
		// Devolvemos erro (o job tenta de novo) em vez de apagar fotos que já temos.
		return fmt.Errorf("nenhuma foto processada: %w", lastErr)
	}
	if err := u.Repo.ReplacePhotos(ctx, job.SpeciesID, photos); err != nil {
		return err
	}
	// Só depois de o banco apontar para as fotos novas apagamos as antigas.
	keep := map[string]bool{}
	for _, p := range photos {
		keep[p.ThumbKey], keep[p.MediumKey], keep[p.LargeKey] = true, true, true
	}
	removeOrphans(ctx, u.Store, photoPrefix(job.SpeciesID), keep)
	return nil
}

func (u IngestPhotos) processOne(ctx context.Context, job Job, a acceptedPhoto) (domain.Photo, error) {
	data, _, err := u.Downloader.Fetch(ctx, a.cand.URL, DownloadImage)
	if err != nil {
		return domain.Photo{}, err
	}
	v, err := u.Images.Process(ctx, data)
	if err != nil {
		return domain.Photo{}, err
	}
	p := domain.Photo{
		ThumbKey:  photoKey(job.SpeciesID, a.cand.SourceID, "thumb"),
		MediumKey: photoKey(job.SpeciesID, a.cand.SourceID, "medium"),
		LargeKey:  photoKey(job.SpeciesID, a.cand.SourceID, "large"),
		Width:     v.Width, Height: v.Height,
		Credit: domain.Credit{Author: a.cand.Author, License: string(a.license), Source: "inaturalist", SourceURL: a.cand.PageURL},
	}
	for key, body := range map[string][]byte{p.ThumbKey: v.Thumb, p.MediumKey: v.Medium, p.LargeKey: v.Large} {
		if err := u.Store.Put(ctx, key, "image/webp", body); err != nil {
			return domain.Photo{}, errors.Join(errors.New("gravar no storage"), err)
		}
	}
	return p, nil
}
