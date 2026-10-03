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

type fakePhotoSource struct {
	cands []ingest.PhotoCandidate
	err   error
}

func (f fakePhotoSource) FindPhotos(context.Context, string, int) ([]ingest.PhotoCandidate, error) {
	return f.cands, f.err
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

type fakeImages struct{ err error }

func (f fakeImages) Process(_ context.Context, orig []byte) (ingest.PhotoVariants, error) {
	if f.err != nil {
		return ingest.PhotoVariants{}, f.err
	}
	return ingest.PhotoVariants{Thumb: []byte("t"), Medium: []byte("m"), Large: []byte("l"), Width: 1600, Height: 1000}, nil
}

// fakeAudio falha para os bytes listados em failFor (para testar o fallback).
type fakeAudio struct{ failFor map[string]bool }

func (f fakeAudio) ToAAC(_ context.Context, orig []byte, _ time.Duration) ([]byte, error) {
	if f.failFor[string(orig)] {
		return nil, errors.New("conversão falhou")
	}
	return []byte("aac:" + string(orig)), nil
}

type fakeStore struct {
	objects map[string]string // key -> contentType
	data    map[string][]byte
	deleted []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{objects: map[string]string{}, data: map[string][]byte{}}
}

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

type fakeRepo struct {
	photos       []domain.Photo
	audio        *domain.Audio
	clusters     []domain.OccurrenceCluster
	photoCalls   int
	audioCalls   int
	clusterCalls int
	replaceErr   error
}

func (f *fakeRepo) ReplacePhotos(_ context.Context, _ string, p []domain.Photo) error {
	f.photoCalls++
	if f.replaceErr != nil {
		return f.replaceErr
	}
	f.photos = p
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

type failCall struct {
	id      int64
	cause   string
	retryIn time.Duration
	giveUp  bool
}

type fakeQueue struct {
	jobs      []ingest.Job
	completed []int64
	failed    []failCall
	released  []int64
	// ctxErrOnFinish guarda ctx.Err() visto por Complete/Fail (deve ser nil).
	ctxErrOnFinish []error
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
