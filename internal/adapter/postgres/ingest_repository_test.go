package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/adapter/postgres"
	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

func TestJobQueue(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	seed(t, repo,
		sp("Turdus rufiventris", "Sabiá-laranjeira", nil),
		sp("Pitangus sulphuratus", "Bem-te-vi", nil))
	q := postgres.NewIngestRepository(pool)

	n, err := q.EnqueueMissing(ctx, ingest.AllSources, 30*24*time.Hour)
	if err != nil || n != 6 {
		t.Fatalf("2 espécies × 3 fontes = 6 pendentes; veio %d, %v", n, err)
	}
	if n, _ := q.EnqueueMissing(ctx, ingest.AllSources, 30*24*time.Hour); n != 6 {
		t.Fatalf("enfileirar de novo não pode duplicar: %d", n)
	}

	jobs, err := q.ClaimDue(ctx, 4)
	if err != nil || len(jobs) != 4 || jobs[0].ScientificName == "" {
		t.Fatalf("claim: %v %+v", err, jobs)
	}
	if again, _ := q.ClaimDue(ctx, 10); len(again) != 2 {
		t.Fatalf("só 2 deveriam sobrar (os 4 estão running), veio %d", len(again))
	}

	if err := q.Fail(ctx, jobs[0].ID, "fonte fora do ar", time.Hour, false); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	_ = pool.QueryRow(ctx, "SELECT status, attempts FROM ingestion_job WHERE id=$1", jobs[0].ID).Scan(&status, &attempts)
	if status != "pending" || attempts != 1 {
		t.Fatalf("depois de Fail: status=%s attempts=%d", status, attempts)
	}
	if due, _ := q.ClaimDue(ctx, 10); len(due) != 0 {
		t.Fatalf("job reagendado para daqui a 1h não pode estar vencido: %+v", due)
	}
	_ = q.Fail(ctx, jobs[1].ID, "x", time.Hour, true)
	_ = pool.QueryRow(ctx, "SELECT status FROM ingestion_job WHERE id=$1", jobs[1].ID).Scan(&status)
	if status != "failed" {
		t.Fatalf("giveUp deveria marcar failed, veio %s", status)
	}
	// Job "preso" em running há mais de 15 min volta para a fila.
	_, _ = pool.Exec(ctx, "UPDATE ingestion_job SET updated_at = now() - interval '1 hour' WHERE id=$1", jobs[3].ID)
	_, _ = q.EnqueueMissing(ctx, ingest.AllSources, 30*24*time.Hour)
	_ = pool.QueryRow(ctx, "SELECT status FROM ingestion_job WHERE id=$1", jobs[3].ID).Scan(&status)
	if status != "pending" {
		t.Fatalf("job preso deveria voltar para pending, veio %s", status)
	}
	_ = q.Complete(ctx, jobs[2].ID)
	_ = pool.QueryRow(ctx, "SELECT status FROM ingestion_job WHERE id=$1", jobs[2].ID).Scan(&status)
	if status != "done" {
		t.Fatalf("Complete: status=%s", status)
	}
}

// Review Focus #5
func TestJobQueue_ClaimDueSkipsLockedJobs(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	seed(t, repo, sp("Turdus rufiventris", "Sabiá-laranjeira", nil), sp("Pitangus sulphuratus", "Bem-te-vi", nil))
	q := postgres.NewIngestRepository(pool)
	_, _ = q.EnqueueMissing(ctx, ingest.AllSources, time.Hour)

	var mu sync.Mutex
	seen := map[int64]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ { // 4 "workers" disputando os mesmos 6 jobs
		wg.Add(1)
		go func() {
			defer wg.Done()
			jobs, err := q.ClaimDue(ctx, 3)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			for _, j := range jobs {
				seen[j.ID]++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("job %d foi pego %d vezes", id, n)
		}
	}
	if len(seen) != 6 {
		t.Fatalf("esperava os 6 jobs distribuídos, veio %d", len(seen))
	}
}

// Review Focus #4
func TestMediaRepository_ReplaceIsIdempotent(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	seed(t, repo, sp("Turdus rufiventris", "Sabiá-laranjeira", nil))
	m := postgres.NewIngestRepository(pool)
	credit := domain.Credit{Author: "A", License: "CC-BY", Source: "inaturalist", SourceURL: "https://www.inaturalist.org/observations/1"}
	photos := []domain.Photo{
		{ThumbKey: "t0", MediumKey: "m0", LargeKey: "l0", Width: 1600, Height: 1000, Credit: credit},
		{ThumbKey: "t1", MediumKey: "m1", LargeKey: "l1", Width: 1600, Height: 900, Credit: credit},
	}
	audio := &domain.Audio{Key: "a.aac", DurationMs: 30000, Credit: domain.Credit{Author: "B", License: "CC-BY-NC-SA", Source: "xeno-canto", SourceURL: "https://xeno-canto.org/1"}}
	clusters := []domain.OccurrenceCluster{{Lat: -23.5, Lng: -46.6, Count: 3, Precision: 1}}
	for i := 0; i < 2; i++ { // duas vezes = reprocessamento
		if err := m.ReplacePhotos(ctx, "turdus-rufiventris", photos); err != nil {
			t.Fatal(err)
		}
		if err := m.ReplaceAudio(ctx, "turdus-rufiventris", audio); err != nil {
			t.Fatal(err)
		}
		if err := m.ReplaceClusters(ctx, "turdus-rufiventris", clusters); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := repo.GetSpecies(ctx, "turdus-rufiventris")
	if len(s.Photos) != 2 || s.Photos[1].LargeKey != "l1" || s.Audio == nil || s.Audio.Key != "a.aac" || len(s.Clusters) != 1 {
		t.Fatalf("depois de 2 replaces: photos=%d audio=%+v clusters=%d", len(s.Photos), s.Audio, len(s.Clusters))
	}
	// Trocar fotos não apaga o áudio (cada fonte cuida do seu tipo).
	_ = m.ReplacePhotos(ctx, "turdus-rufiventris", photos[:1])
	s, _ = repo.GetSpecies(ctx, "turdus-rufiventris")
	if len(s.Photos) != 1 || s.Audio == nil {
		t.Fatalf("replace de fotos afetou o áudio: %+v", s)
	}
	// Áudio nil = "não há canto": remove o anterior.
	_ = m.ReplaceAudio(ctx, "turdus-rufiventris", nil)
	s, _ = repo.GetSpecies(ctx, "turdus-rufiventris")
	if s.Audio != nil {
		t.Fatal("ReplaceAudio(nil) deveria remover o canto")
	}
}
