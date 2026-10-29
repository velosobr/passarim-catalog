// Testes do caso de uso de fotos com o curador: licença, download, curadoria,
// escolha, override, relatório e storage, todos com peças falsas.

package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// sabia é o job de fotos usado nos testes.
var sabia = ingest.Job{ID: 1, SpeciesID: "turdus-rufiventris", ScientificName: "Turdus rufiventris", Source: ingest.SourceINaturalist}

// cand cria a candidata de id n, cada uma na sua observação (n+1000).
func cand(n int, license string) ingest.PhotoCandidate {
	return ingest.PhotoCandidate{SourceID: fmt.Sprint(n), ObservationID: fmt.Sprint(n + 1000),
		URL: fmt.Sprintf("https://x/%d.jpg", n), PageURL: fmt.Sprintf("https://www.inaturalist.org/observations/%d", n+1000),
		License: license, Author: "Autor", Width: 2000, Height: 1000}
}

// cands cria as candidatas de 0 a n-1, todas cc-by.
func cands(n int) []ingest.PhotoCandidate {
	var out []ingest.PhotoCandidate
	for i := range n {
		out = append(out, cand(i, "cc-by"))
	}
	return out
}

// photoFiles monta o downloader falso: a foto n devolve "img-n".
func photoFiles(ids ...int) map[string][]byte {
	m := map[string][]byte{}
	for _, i := range ids {
		m[fmt.Sprintf("https://x/%d.jpg", i)] = []byte(fmt.Sprintf("img-%d", i))
	}
	return m
}

// upTo devolve 0, 1, ..., n-1.
func upTo(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// env junta as peças falsas de um teste.
type env struct {
	uc      ingest.IngestPhotos
	dl      *fakeDownloader
	imgs    *fakeImages
	curator *fakeCurator
	store   *fakeStore
	repo    *fakeRepo
}

// newEnv monta o caso de uso como o worker (6 fotos, 40 candidatas, 20
// observações), com o downloader respondendo às fotos dadas.
func newEnv(src fakePhotoSource, files map[string][]byte) *env {
	e := &env{dl: &fakeDownloader{files: files}, imgs: &fakeImages{}, curator: &fakeCurator{},
		store: newFakeStore(), repo: &fakeRepo{}}
	e.uc = ingest.IngestPhotos{Source: src, Downloader: e.dl, Images: e.imgs, Curator: e.curator, Store: e.store,
		Repo: e.repo, Overrides: fakeOverrides{}, Policy: domain.LicensePolicy{AllowNC: true},
		MaxPhotos: 6, MaxCandidates: 40, Observations: 20, LowConfidence: 0.3, Timeout: time.Minute}
	return e
}

// gallery devolve os ids de origem das fotos gravadas, na ordem.
func (e *env) gallery() string {
	var out []string
	for _, p := range e.repo.photos {
		out = append(out, p.SourcePhotoID)
	}
	return strings.Join(out, ",")
}

// A principal é a de maior pontuação, e cada foto gravada leva categoria,
// pontuação, versão do modelo e as três chaves.
func TestIngestPhotos_PicksPrimaryByScore(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(3)}, photoFiles(0, 1, 2))
	e.curator.verdicts = map[string]ingest.PhotoVerdict{
		"2": {Category: domain.PhotoSingleBird, LargestBirdArea: 0.3, Sharpness: 0.9, Confidence: 0.9},
	}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if e.gallery() != "2,0,1" {
		t.Fatalf("galeria: %s", e.gallery())
	}
	p := e.repo.photos[0]
	if p.Category != domain.PhotoSingleBird || p.ModelVersion != "fake@1" || p.Score <= e.repo.photos[1].Score ||
		p.LargeKey != "species/turdus-rufiventris/photo-2-large.webp" || p.Credit.License != "CC-BY" {
		t.Fatalf("principal gravada: %+v", p)
	}
	if e.repo.report.PrimaryPhotoID != "2" || len(e.repo.report.Candidates) != 3 || e.repo.report.ModelVersion != "fake@1" {
		t.Fatalf("relatório: %+v", e.repo.report)
	}
}

// As candidatas vão ao curador em lotes de até 8, e são cortadas em 40.
func TestIngestPhotos_BatchesOfEightAndCutAtForty(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(45)}, photoFiles(upTo(45)...))
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(e.curator.batches) != "[8 8 8 8 8]" {
		t.Fatalf("lotes: %v (40 candidatas em lotes de 8)", e.curator.batches)
	}
	if len(e.dl.requested) != 40 {
		t.Fatalf("baixou %d, esperava 40", len(e.dl.requested))
	}
}

// Só as escolhidas ganham thumb e large e vão para o storage (3 arquivos cada).
func TestIngestPhotos_OnlyChosenAreStored(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(10)}, photoFiles(upTo(10)...))
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if len(e.repo.photos) != 6 || e.imgs.thumbAndLarge != 6 || len(e.store.objects) != 18 {
		t.Fatalf("fotos=%d thumb+large=%d objetos=%d", len(e.repo.photos), e.imgs.thumbAndLarge, len(e.store.objects))
	}
}

// Curador fora do ar: ErrCuratorUnavailable, nada gravado no banco nem no storage.
func TestIngestPhotos_CuratorUnavailable(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(2)}, photoFiles(0, 1))
	e.curator.err = fmt.Errorf("conexão: %w", ingest.ErrCuratorUnavailable)
	e.store.objects["species/turdus-rufiventris/photo-OLD-thumb.webp"] = "x"
	err := e.uc.Run(context.Background(), sabia)
	if !errors.Is(err, ingest.ErrCuratorUnavailable) {
		t.Fatalf("esperava ErrCuratorUnavailable, veio %v", err)
	}
	if e.repo.photoCalls != 0 || len(e.store.deleted) != 0 || len(e.store.objects) != 1 {
		t.Fatalf("nada podia mudar: calls=%d deleted=%v objects=%v", e.repo.photoCalls, e.store.deleted, e.store.objects)
	}
}

// Uma foto UNSPECIFIED (não abriu no curador) é descarte.
func TestIngestPhotos_UnspecifiedIsDiscarded(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(2)}, photoFiles(0, 1))
	e.curator.verdicts = map[string]ingest.PhotoVerdict{"0": {Category: domain.PhotoUnspecified}}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if e.gallery() != "1" {
		t.Fatalf("galeria: %s", e.gallery())
	}
}

// Só bandos: a espécie fica sem fotos, os arquivos antigos saem e o
// relatório marca no_single_bird.
func TestIngestPhotos_OnlyFlocksLeavesNoPhotos(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(2)}, photoFiles(0, 1))
	e.curator.verdicts = map[string]ingest.PhotoVerdict{"0": {Category: domain.PhotoFlock}, "1": {Category: domain.PhotoFlock}}
	e.store.objects["species/turdus-rufiventris/photo-OLD-thumb.webp"] = "x"
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if e.repo.photoCalls != 1 || len(e.repo.photos) != 0 || len(e.store.objects) != 0 {
		t.Fatalf("esperava nenhuma foto e storage limpo: %+v %v", e.repo.photos, e.store.objects)
	}
	if e.repo.report.PrimaryPhotoID != "" || len(e.repo.report.Flags) != 1 || e.repo.report.Flags[0] != domain.FlagNoSingleBird {
		t.Fatalf("relatório: %+v", e.repo.report)
	}
}

// photos.exclude tira a foto, mesmo sendo a melhor.
func TestIngestPhotos_OverrideExclude(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(2)}, photoFiles(0, 1))
	e.curator.verdicts = map[string]ingest.PhotoVerdict{"0": {Category: domain.PhotoSingleBird, LargestBirdArea: 1, Sharpness: 1, Confidence: 1}}
	e.uc.Overrides = fakeOverrides{domain.PhotoOverride{ExcludedIDs: []string{"0"}}}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if e.gallery() != "1" || slicesContain(e.dl.requested, "https://x/0.jpg") {
		t.Fatalf("galeria: %s, baixou: %v", e.gallery(), e.dl.requested)
	}
}

// photos.primary passa à frente, mesmo fora das candidatas e sendo bando.
func TestIngestPhotos_OverridePrimary(t *testing.T) {
	primary := cand(99, "cc-by")
	e := newEnv(fakePhotoSource{cands: cands(2), primary: map[string]ingest.PhotoCandidate{"99": primary}}, photoFiles(0, 1, 99))
	e.curator.verdicts = map[string]ingest.PhotoVerdict{"99": {Category: domain.PhotoFlock}}
	e.uc.Overrides = fakeOverrides{domain.PhotoOverride{PrimaryObservationID: "1099", PrimaryPhotoID: "99"}}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if e.gallery() != "99,0,1" || e.repo.report.PrimaryPhotoID != "99" || len(e.repo.report.Flags) != 0 {
		t.Fatalf("galeria: %s, relatório: %+v", e.gallery(), e.repo.report)
	}
}

// A principal do override que também está nas candidatas entra uma vez só.
func TestIngestPhotos_OverridePrimaryNotDuplicated(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(3), primary: map[string]ingest.PhotoCandidate{"2": cand(2, "cc-by")}}, photoFiles(0, 1, 2))
	e.uc.Overrides = fakeOverrides{domain.PhotoOverride{PrimaryObservationID: "1002", PrimaryPhotoID: "2"}}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if e.gallery() != "2,0,1" || len(e.repo.report.Candidates) != 3 {
		t.Fatalf("galeria: %s, candidatas no relatório: %d", e.gallery(), len(e.repo.report.Candidates))
	}
}

// A foto do override sumiu: a principal volta a ser a do modelo, com override_failed.
func TestIngestPhotos_OverrideMissingFails(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(2)}, photoFiles(0, 1))
	e.uc.Overrides = fakeOverrides{domain.PhotoOverride{PrimaryObservationID: "1", PrimaryPhotoID: "77"}}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if e.gallery() != "0,1" || !hasFlag(e.repo.report, domain.FlagOverrideFailed) {
		t.Fatalf("galeria: %s, relatório: %+v", e.gallery(), e.repo.report)
	}
}

// A foto do override mudou para uma licença fora da política (NC com
// ALLOW_NC=false): override_failed, e a foto não é baixada.
func TestIngestPhotos_OverrideRejectedLicenseFails(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(1), primary: map[string]ingest.PhotoCandidate{"99": cand(99, "cc-by-nc")}}, photoFiles(0, 99))
	e.uc.Policy = domain.LicensePolicy{AllowNC: false}
	e.uc.Overrides = fakeOverrides{domain.PhotoOverride{PrimaryObservationID: "1099", PrimaryPhotoID: "99"}}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if e.gallery() != "0" || !hasFlag(e.repo.report, domain.FlagOverrideFailed) || slicesContain(e.dl.requested, "https://x/99.jpg") {
		t.Fatalf("galeria: %s, relatório: %+v, baixou %v", e.gallery(), e.repo.report, e.dl.requested)
	}
}

// A foto do override não abriu (download falhou ou o curador deu UNSPECIFIED): override_failed.
func TestIngestPhotos_OverrideBrokenFails(t *testing.T) {
	for name, setup := range map[string]func(e *env){
		"download": func(e *env) { delete(e.dl.files, "https://x/99.jpg") },
		"curador": func(e *env) {
			e.curator.verdicts = map[string]ingest.PhotoVerdict{"99": {Category: domain.PhotoUnspecified}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(fakePhotoSource{cands: cands(1), primary: map[string]ingest.PhotoCandidate{"99": cand(99, "cc-by")}}, photoFiles(0, 99))
			e.uc.Overrides = fakeOverrides{domain.PhotoOverride{PrimaryObservationID: "1099", PrimaryPhotoID: "99"}}
			setup(e)
			if err := e.uc.Run(context.Background(), sabia); err != nil {
				t.Fatal(err)
			}
			if e.gallery() != "0" || !hasFlag(e.repo.report, domain.FlagOverrideFailed) {
				t.Fatalf("galeria: %s, relatório: %+v", e.gallery(), e.repo.report)
			}
		})
	}
}

// A principal com confiança baixa é marcada low_confidence.
func TestIngestPhotos_LowConfidenceFlag(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(1)}, photoFiles(0))
	e.curator.verdicts = map[string]ingest.PhotoVerdict{"0": {Category: domain.PhotoSingleBird, Confidence: 0.1}}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if !hasFlag(e.repo.report, domain.FlagLowConfidence) {
		t.Fatalf("relatório: %+v", e.repo.report)
	}
}

// Fotos com licença não aceita são ignoradas antes do download; sem nenhuma
// sobrando, as fotos da espécie são limpas e o relatório registra.
func TestIngestPhotos_SkipsRejectedLicenses(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: []ingest.PhotoCandidate{cand(0, "cc-by-nd"), cand(1, ""), cand(2, "cc-by-nc")}}, photoFiles(0, 1, 2))
	e.uc.Policy = domain.LicensePolicy{AllowNC: false}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if len(e.dl.requested) != 0 || e.repo.photoCalls != 1 || len(e.repo.photos) != 0 || len(e.curator.batches) != 0 {
		t.Fatalf("baixou %v, calls=%d, lotes=%v", e.dl.requested, e.repo.photoCalls, e.curator.batches)
	}
	if !hasFlag(e.repo.report, domain.FlagNoSingleBird) {
		t.Fatalf("relatório: %+v", e.repo.report)
	}
}

// Um id da fonte com caracteres perigosos é ignorado.
func TestIngestPhotos_SkipsUnsafeSourceID(t *testing.T) {
	bad := cand(0, "cc-by")
	bad.SourceID = "../../outro"
	e := newEnv(fakePhotoSource{cands: []ingest.PhotoCandidate{bad}}, photoFiles(0))
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if len(e.dl.requested) != 0 || len(e.store.objects) != 0 {
		t.Fatalf("id inseguro não pode virar chave: dl=%v store=%v", e.dl.requested, e.store.objects)
	}
}

// Uma foto que falha no download ou na medium não derruba as outras.
func TestIngestPhotos_SkipsBrokenPhoto(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(3)}, photoFiles(1, 2)) // a 0 falha no download
	e.imgs.failMedium = map[string]bool{"img-1": true}              // a 1 falha na medium
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if e.gallery() != "2" {
		t.Fatalf("galeria: %s", e.gallery())
	}
}

// Se todas falham no download, o job dá erro e as fotos antigas ficam.
func TestIngestPhotos_AllDownloadsFailIsError(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(2)}, map[string][]byte{})
	if err := e.uc.Run(context.Background(), sabia); err == nil || errors.Is(err, ingest.ErrCuratorUnavailable) {
		t.Fatalf("falha total deveria virar erro comum, veio %v", err)
	}
	if e.repo.photoCalls != 0 {
		t.Fatal("não pode apagar fotos boas por causa de uma falha temporária")
	}
}

// Erro da fonte vira erro do job.
func TestIngestPhotos_SourceErrorIsError(t *testing.T) {
	e := newEnv(fakePhotoSource{err: errors.New("fora do ar")}, nil)
	if err := e.uc.Run(context.Background(), sabia); err == nil || e.repo.photoCalls != 0 {
		t.Fatalf("erro da fonte deveria propagar sem gravar: %v", err)
	}
}

// Depois de trocar as fotos no banco, os arquivos antigos saem do storage;
// o canto e as outras espécies ficam.
func TestIngestPhotos_DeletesOrphanPhotosAfterReplace(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(2)}, photoFiles(0, 1))
	for _, k := range []string{"species/turdus-rufiventris/photo-OLD-thumb.webp", "species/turdus-rufiventris/audio-7.aac",
		"species/outra-especie/photo-OLD-thumb.webp"} {
		e.store.objects[k] = "x"
	}
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.store.objects["species/turdus-rufiventris/photo-OLD-thumb.webp"]; ok {
		t.Fatal("foto antiga órfã deveria ser apagada")
	}
	for _, k := range []string{"species/turdus-rufiventris/audio-7.aac", "species/outra-especie/photo-OLD-thumb.webp",
		"species/turdus-rufiventris/photo-1-medium.webp"} {
		if _, ok := e.store.objects[k]; !ok {
			t.Fatalf("%s não podia ser apagado", k)
		}
	}
}

// Se a troca no banco falha, nada é apagado do storage.
func TestIngestPhotos_NoCleanupWhenReplaceFails(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(1)}, photoFiles(0))
	e.repo.replaceErr = errors.New("banco fora")
	e.store.objects["species/turdus-rufiventris/photo-OLD-thumb.webp"] = "x"
	if err := e.uc.Run(context.Background(), sabia); err == nil {
		t.Fatal("falha do banco deveria propagar")
	}
	if len(e.store.deleted) != 0 {
		t.Fatalf("o banco ainda referencia a foto antiga: não pode apagar %v", e.store.deleted)
	}
}

// 40 candidatas com curador e conversor falsos lentos terminam dentro do prazo.
func TestIngestPhotos_FinishesWithinTimeout(t *testing.T) {
	e := newEnv(fakePhotoSource{cands: cands(40)}, photoFiles(upTo(40)...))
	e.imgs.delay, e.curator.delay = 5*time.Millisecond, 100*time.Millisecond
	e.uc.Timeout = 3 * time.Second
	start := time.Now()
	if err := e.uc.Run(context.Background(), sabia); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > e.uc.Timeout {
		t.Fatalf("levou %v", time.Since(start))
	}
}

// hasFlag diz se o relatório tem a marca.
func hasFlag(r domain.CurationReport, f domain.CurationFlag) bool {
	for _, x := range r.Flags {
		if x == f {
			return true
		}
	}
	return false
}

// slicesContain diz se a lista tem o texto.
func slicesContain(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
