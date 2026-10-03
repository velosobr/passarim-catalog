package ingest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

var sabia = ingest.Job{ID: 1, SpeciesID: "turdus-rufiventris", ScientificName: "Turdus rufiventris", Source: ingest.SourceINaturalist}

func cand(n int, license string) ingest.PhotoCandidate {
	return ingest.PhotoCandidate{URL: fmt.Sprintf("https://x/%d.jpg", n), PageURL: fmt.Sprintf("https://www.inaturalist.org/observations/%d", n),
		License: license, Author: "Autor", Width: 2000, Height: 1000}
}

func photoFiles(n int) map[string][]byte {
	m := map[string][]byte{}
	for i := 0; i < n; i++ {
		m[fmt.Sprintf("https://x/%d.jpg", i)] = []byte("img")
	}
	return m
}

func newPhotos(src fakePhotoSource, dl *fakeDownloader, imgs fakeImages, policy domain.LicensePolicy) (ingest.IngestPhotos, *fakeStore, *fakeRepo) {
	store, repo := newFakeStore(), &fakeRepo{}
	return ingest.IngestPhotos{Source: src, Downloader: dl, Images: imgs, Store: store, Repo: repo, Policy: policy, MaxPhotos: 5}, store, repo
}

func TestIngestPhotos_StoresVariantsAndCredits(t *testing.T) {
	uc, store, repo := newPhotos(fakePhotoSource{cands: []ingest.PhotoCandidate{cand(0, "cc-by"), cand(1, "cc-by")}},
		&fakeDownloader{files: photoFiles(2)}, fakeImages{}, domain.LicensePolicy{AllowNC: true})
	if err := uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if len(store.objects) != 6 {
		t.Fatalf("esperava 6 objetos, veio %d: %v", len(store.objects), store.objects)
	}
	for _, k := range []string{"species/turdus-rufiventris/photo-0-thumb.webp", "species/turdus-rufiventris/photo-1-large.webp"} {
		if store.objects[k] != "image/webp" {
			t.Errorf("objeto %s ausente ou com tipo errado: %q", k, store.objects[k])
		}
	}
	if len(repo.photos) != 2 || repo.photos[0].Credit.License != "CC-BY" || repo.photos[0].Credit.Source != "inaturalist" ||
		repo.photos[0].Credit.SourceURL != "https://www.inaturalist.org/observations/0" || repo.photos[0].LargeKey != "species/turdus-rufiventris/photo-0-large.webp" {
		t.Fatalf("fotos gravadas: %+v", repo.photos)
	}
}

// Review Focus #2
func TestIngestPhotos_SkipsRejectedLicenses(t *testing.T) {
	dl := &fakeDownloader{files: photoFiles(3)}
	uc, _, repo := newPhotos(fakePhotoSource{cands: []ingest.PhotoCandidate{cand(0, "cc-by-nd"), cand(1, ""), cand(2, "cc-by-nc")}},
		dl, fakeImages{}, domain.LicensePolicy{AllowNC: false})
	if err := uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if len(dl.requested) != 0 {
		t.Fatalf("nenhuma URL deveria ser baixada, pediu %v", dl.requested)
	}
	if repo.photoCalls != 1 || repo.photos != nil {
		t.Fatalf("esperava ReplacePhotos(nil) uma vez: calls=%d photos=%v", repo.photoCalls, repo.photos)
	}
}

func TestIngestPhotos_RespectsMax(t *testing.T) {
	var cands []ingest.PhotoCandidate
	for i := 0; i < 10; i++ {
		cands = append(cands, cand(i, "cc0"))
	}
	uc, _, repo := newPhotos(fakePhotoSource{cands: cands}, &fakeDownloader{files: photoFiles(10)}, fakeImages{}, domain.LicensePolicy{AllowNC: true})
	if err := uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if len(repo.photos) != 5 {
		t.Fatalf("esperava 5 fotos, veio %d", len(repo.photos))
	}
}

func TestIngestPhotos_SkipsBrokenPhoto(t *testing.T) {
	files := photoFiles(2)
	delete(files, "https://x/0.jpg") // a primeira falha no download
	uc, store, repo := newPhotos(fakePhotoSource{cands: []ingest.PhotoCandidate{cand(0, "cc-by"), cand(1, "cc-by")}},
		&fakeDownloader{files: files}, fakeImages{}, domain.LicensePolicy{AllowNC: true})
	if err := uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if len(repo.photos) != 1 || repo.photos[0].ThumbKey != "species/turdus-rufiventris/photo-0-thumb.webp" || len(store.objects) != 3 {
		t.Fatalf("a foto boa deve ficar na posição 0: %+v", repo.photos)
	}
}

func TestIngestPhotos_AllDownloadsFailIsError(t *testing.T) {
	uc, _, repo := newPhotos(fakePhotoSource{cands: []ingest.PhotoCandidate{cand(0, "cc-by"), cand(1, "cc-by")}},
		&fakeDownloader{files: map[string][]byte{}}, fakeImages{}, domain.LicensePolicy{AllowNC: true})
	if err := uc.Run(context.Background(), sabia); err == nil {
		t.Fatal("falha total deveria virar erro (o job tenta de novo)")
	}
	if repo.photoCalls != 0 {
		t.Fatal("não pode apagar fotos boas por causa de uma falha temporária")
	}
}

func TestIngestPhotos_SourceErrorIsError(t *testing.T) {
	uc, _, repo := newPhotos(fakePhotoSource{err: fmt.Errorf("fora do ar")}, &fakeDownloader{}, fakeImages{}, domain.LicensePolicy{})
	if err := uc.Run(context.Background(), sabia); err == nil || repo.photoCalls != 0 {
		t.Fatalf("erro da fonte deveria propagar sem gravar: %v", err)
	}
}
