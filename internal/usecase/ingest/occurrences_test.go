package ingest_test

import (
	"context"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

func TestIngestOccurrences_ClustersAndReplaces(t *testing.T) {
	repo := &fakeRepo{}
	uc := ingest.IngestOccurrences{Source: fakeOccSource{points: []domain.Point{
		{Lat: -23.5, Lng: -46.6}, {Lat: -23.7, Lng: -46.2}, {Lat: -23.1, Lng: -46.9}, {Lat: -22.9, Lng: -43.2},
	}}, Repo: repo, MaxPoints: 900, CellDeg: 1.0}
	if err := uc.Run(context.Background(), ingest.Job{SpeciesID: "x-y", ScientificName: "X y"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.clusters) != 2 || repo.clusters[0].Precision != 1.0 || repo.clusters[0].Count != 3 {
		t.Fatalf("clusters: %+v", repo.clusters)
	}
}

func TestIngestOccurrences_NoPointsWritesEmpty(t *testing.T) {
	repo := &fakeRepo{}
	uc := ingest.IngestOccurrences{Source: fakeOccSource{}, Repo: repo, MaxPoints: 900, CellDeg: 1.0}
	if err := uc.Run(context.Background(), ingest.Job{SpeciesID: "x-y"}); err != nil || repo.clusterCalls != 1 || len(repo.clusters) != 0 {
		t.Fatalf("zero pontos deveria gravar lista vazia: %v calls=%d", err, repo.clusterCalls)
	}
}
