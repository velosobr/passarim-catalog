package domain

import (
	"math"
	"sort"
)

// Point é um avistamento (latitude/longitude em graus).
type Point struct {
	Lat, Lng float64
}

// ClusterPoints agrupa pontos numa grade de cellDeg graus. Cada célula vira
// UM ponto (a média das coordenadas) com a contagem. Assim o app desenha
// dezenas de círculos em vez de milhares de marcadores.
func ClusterPoints(points []Point, cellDeg float64) []OccurrenceCluster {
	type acc struct {
		sumLat, sumLng float64
		n              int
	}
	cells := map[[2]int]*acc{}
	for _, p := range points {
		// math.Floor (e não int()) para números negativos: int(-0.5) = 0,
		// mas -0.5 pertence à célula -1.
		key := [2]int{int(math.Floor(p.Lat / cellDeg)), int(math.Floor(p.Lng / cellDeg))}
		a := cells[key]
		if a == nil {
			a = &acc{}
			cells[key] = a
		}
		a.sumLat += p.Lat
		a.sumLng += p.Lng
		a.n++
	}
	out := make([]OccurrenceCluster, 0, len(cells))
	for _, a := range cells {
		out = append(out, OccurrenceCluster{Lat: a.sumLat / float64(a.n), Lng: a.sumLng / float64(a.n), Count: a.n, Precision: cellDeg})
	}
	// Ordem determinística (mapas em Go não têm ordem).
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Lat != out[j].Lat {
			return out[i].Lat < out[j].Lat
		}
		return out[i].Lng < out[j].Lng
	})
	return out
}
