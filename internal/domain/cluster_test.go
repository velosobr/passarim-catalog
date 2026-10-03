package domain_test

import (
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

func TestClusterPoints(t *testing.T) {
	pts := []domain.Point{
		{Lat: -23.5, Lng: -46.6}, {Lat: -23.7, Lng: -46.2}, {Lat: -23.1, Lng: -46.9}, // mesma célula (SP)
		{Lat: -22.9, Lng: -43.2}, // outra célula (RJ)
	}
	got := domain.ClusterPoints(pts, 1.0)
	if len(got) != 2 {
		t.Fatalf("esperava 2 clusters, veio %d: %+v", len(got), got)
	}
	sp := got[0] // o maior vem primeiro
	if sp.Count != 3 || sp.Precision != 1.0 {
		t.Fatalf("cluster SP = %+v", sp)
	}
	// Centróide = média dos pontos da célula.
	if d := sp.Lat - (-23.5-23.7-23.1)/3; d > 1e-9 || d < -1e-9 {
		t.Fatalf("lat do centróide = %v", sp.Lat)
	}
	if got[1].Count != 1 {
		t.Fatalf("cluster RJ = %+v", got[1])
	}
}

func TestClusterPoints_Empty(t *testing.T) {
	if got := domain.ClusterPoints(nil, 1.0); len(got) != 0 {
		t.Fatalf("sem pontos deveria dar lista vazia, veio %v", got)
	}
}

func TestClusterPoints_NegativeCoordinatesUseFloor(t *testing.T) {
	// -0.5 e +0.5 estão em células DIFERENTES (floor(-0.5) = -1, floor(0.5) = 0).
	got := domain.ClusterPoints([]domain.Point{{Lat: -0.5, Lng: 0}, {Lat: 0.5, Lng: 0}}, 1.0)
	if len(got) != 2 {
		t.Fatalf("pontos dos dois lados do equador caíram juntos: %+v", got)
	}
}
