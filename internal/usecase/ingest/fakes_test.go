// Peças falsas dos testes de ingestão.

package ingest_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// Fakes simples e escritos à mão: guardam o que receberam para o teste conferir.

// As três fontes falsas devolvem a lista e o erro configurados.
// fakePhotoSource devolve as candidatas e o erro configurados. primary é a
// resposta do FindObservationPhoto, por id da foto (ausente = não achou).
type fakePhotoSource struct {
	cands   []ingest.PhotoCandidate
	err     error
	primary map[string]ingest.PhotoCandidate
}

func (f fakePhotoSource) FindPhotos(context.Context, string, int) ([]ingest.PhotoCandidate, error) {
	return f.cands, f.err
}

func (f fakePhotoSource) FindObservationPhoto(_ context.Context, _, photoID string) (ingest.PhotoCandidate, bool, error) {
	c, ok := f.primary[photoID]
	return c, ok, nil
}

type fakeAudioSource struct {
	cands []ingest.AudioCandidate
	err   error
}

func (f fakeAudioSource) FindRecordings(context.Context, string) ([]ingest.AudioCandidate, error) {
	return f.cands, f.err
}

type fakeOccSource struct {
	points []domain.Point
	err    error
}

func (f fakeOccSource) FindOccurrences(context.Context, string, int) ([]domain.Point, error) {
	return f.points, f.err
}

// fakeDownloader devolve bytes por URL; URL ausente do mapa = erro.
type fakeDownloader struct {
	mu        sync.Mutex
	files     map[string][]byte
	requested []string
}

func (f *fakeDownloader) Fetch(_ context.Context, url string, _ ingest.DownloadKind) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requested = append(f.requested, url)
	b, ok := f.files[url]
	if !ok {
		return nil, "", errors.New("download falhou")
	}
	return b, "image/jpeg", nil
}

// fakeImages "converte" sem ffmpeg: a medium é "m:" + os bytes originais
// (o curador falso usa isso para saber de que foto se trata); thumb e
// large são fixas, 1600×1000. failMedium faz a medium falhar para os
// originais listados; thumbAndLarge conta as chamadas.
type fakeImages struct {
	err           error
	failMedium    map[string]bool
	delay         time.Duration
	mu            sync.Mutex
	thumbAndLarge int
}

func (f *fakeImages) Medium(_ context.Context, orig []byte) ([]byte, error) {
	time.Sleep(f.delay)
	if f.err != nil || f.failMedium[string(orig)] {
		return nil, errors.New("conversão falhou")
	}
	return []byte("m:" + string(orig)), nil
}

func (f *fakeImages) ThumbAndLarge(_ context.Context, _ []byte) (ingest.PhotoVariants, error) {
	time.Sleep(f.delay)
	f.mu.Lock()
	f.thumbAndLarge++
	f.mu.Unlock()
	if f.err != nil {
		return ingest.PhotoVariants{}, f.err
	}
	return ingest.PhotoVariants{Thumb: []byte("t"), Large: []byte("l"), Width: 1600, Height: 1000}, nil
}

// fakeAudio falha para os bytes listados em failFor (para testar o fallback).
type fakeAudio struct{ failFor map[string]bool }

func (f fakeAudio) ToAAC(_ context.Context, orig []byte, _ time.Duration) ([]byte, error) {
	if f.failFor[string(orig)] {
		return nil, errors.New("conversão falhou")
	}
	return []byte("aac:" + string(orig)), nil
}

// fakeStore é um object storage em memória; guarda o que foi gravado e a lista
// do que foi apagado.
type fakeStore struct {
	objects map[string]string // key -> contentType
	data    map[string][]byte
	deleted []string
}

// newFakeStore cria o storage vazio.
func newFakeStore() *fakeStore {
	return &fakeStore{objects: map[string]string{}, data: map[string][]byte{}}
}

// List, Delete e Put imitam o storage real sobre o mapa.
func (f *fakeStore) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (f *fakeStore) Delete(_ context.Context, keys []string) error {
	f.deleted = append(f.deleted, keys...)
	for _, k := range keys {
		delete(f.objects, k)
		delete(f.data, k)
	}
	return nil
}

func (f *fakeStore) Put(_ context.Context, key, ct string, data []byte) error {
	f.objects[key] = ct
	f.data[key] = data
	return nil
}

// fakeRepo guarda o último valor gravado de cada tipo.
type fakeRepo struct {
	photos       []domain.Photo
	report       domain.CurationReport
	audio        *domain.Audio
	clusters     []domain.OccurrenceCluster
	photoCalls   int
	audioCalls   int
	clusterCalls int
	replaceErr   error
}

// Os três Replace* contam a chamada e guardam o valor recebido. ReplacePhotos
// e ReplaceAudio devolvem replaceErr, se configurado; ReplaceClusters nunca falha.
func (f *fakeRepo) ReplacePhotos(_ context.Context, _ string, p []domain.Photo, r domain.CurationReport) error {
	f.photoCalls++
	if f.replaceErr != nil {
		return f.replaceErr
	}
	f.photos, f.report = p, r
	return nil
}

func (f *fakeRepo) ReplaceAudio(_ context.Context, _ string, a *domain.Audio) error {
	f.audioCalls++
	if f.replaceErr != nil {
		return f.replaceErr
	}
	f.audio = a
	return nil
}

func (f *fakeRepo) ReplaceClusters(_ context.Context, _ string, c []domain.OccurrenceCluster) error {
	f.clusterCalls++
	f.clusters = c
	return nil
}

// failCall registra uma chamada a Fail da fila falsa.
type failCall struct {
	id      int64
	cause   string
	retryIn time.Duration
	giveUp  bool
}

// fakeQueue é a fila falsa: ClaimDue devolve os jobs configurados (até o
// limite), e Complete, Fail e Release só registram o que receberam e com que
// contexto, para o teste conferir.
type fakeQueue struct {
	jobs      []ingest.Job
	completed []int64
	failed    []failCall
	released  []int64
	deferred  []failCall
	// ctxErrOnFinish guarda ctx.Err() visto por Complete/Fail (deve ser nil).
	ctxErrOnFinish []error
}

func (f *fakeQueue) Defer(ctx context.Context, id int64, cause string, retryIn time.Duration) error {
	f.ctxErrOnFinish = append(f.ctxErrOnFinish, ctx.Err())
	f.deferred = append(f.deferred, failCall{id: id, cause: cause, retryIn: retryIn})
	return nil
}

func (f *fakeQueue) EnqueueMissing(context.Context, []ingest.Source, time.Duration) (int, error) {
	return len(f.jobs), nil
}

func (f *fakeQueue) ClaimDue(_ context.Context, limit int) ([]ingest.Job, error) {
	if limit < len(f.jobs) {
		return f.jobs[:limit], nil
	}
	return f.jobs, nil
}

func (f *fakeQueue) Complete(ctx context.Context, id int64) error {
	f.ctxErrOnFinish = append(f.ctxErrOnFinish, ctx.Err())
	f.completed = append(f.completed, id)
	return nil
}

func (f *fakeQueue) Fail(ctx context.Context, id int64, cause string, retryIn time.Duration, giveUp bool) error {
	f.ctxErrOnFinish = append(f.ctxErrOnFinish, ctx.Err())
	f.failed = append(f.failed, failCall{id, cause, retryIn, giveUp})
	return nil
}

func (f *fakeQueue) Release(_ context.Context, id int64) error {
	f.released = append(f.released, id)
	return nil
}

// fakeCurator classifica pela medium: a medium do fakeImages é "m:" + os
// bytes originais, e os originais dos testes são "img-<id>". verdicts dá o
// veredito de cada id; sem entrada, a foto é uma ave sozinha comum. err faz
// todas as chamadas falharem; batches guarda o tamanho de cada lote.
type fakeCurator struct {
	verdicts map[string]ingest.PhotoVerdict
	err      error
	delay    time.Duration
	mu       sync.Mutex
	batches  []int
}

func (f *fakeCurator) ClassifyPhotos(_ context.Context, photos []ingest.CuratorPhoto) ([]ingest.PhotoVerdict, string, error) {
	time.Sleep(f.delay)
	f.mu.Lock()
	f.batches = append(f.batches, len(photos))
	f.mu.Unlock()
	if f.err != nil {
		return nil, "", f.err
	}
	out := make([]ingest.PhotoVerdict, len(photos))
	for i, p := range photos {
		id := strings.TrimPrefix(string(p.Image), "m:img-")
		v, ok := f.verdicts[id]
		if !ok {
			v = ingest.PhotoVerdict{Category: domain.PhotoSingleBird, LargestBirdArea: 0.1, Sharpness: 0.5, Confidence: 0.5}
		}
		v.PhotoID = p.PhotoID
		out[i] = v
	}
	return out, "fake@1", nil
}

// fakeOverrides devolve o override configurado para qualquer espécie.
type fakeOverrides struct{ o domain.PhotoOverride }

func (f fakeOverrides) PhotoOverride(context.Context, string) (domain.PhotoOverride, error) {
	return f.o, nil
}
