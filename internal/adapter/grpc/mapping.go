package grpcadapter

import (
	"math"

	catalogv1 "github.com/velosobr/passarim-proto/gen/go/passarim/catalog/v1"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// toInt32 converte um int para int32 saturando nos limites do int32 em vez
// de dar overflow silencioso (gosec G115). Os valores convertidos aqui vêm
// de conteúdo curado (tamanho em cm, dimensões de foto, duração de áudio,
// contagem de ocorrências), então a saturação nunca deve ocorrer na prática.
func toInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// Tabelas de tradução entre o mundo do domínio (strings) e o do proto (enums).
var biomeToProto = map[domain.Biome]catalogv1.Biome{
	domain.BiomeAmazonia: catalogv1.Biome_BIOME_AMAZONIA, domain.BiomeMataAtlantica: catalogv1.Biome_BIOME_MATA_ATLANTICA,
	domain.BiomeCerrado: catalogv1.Biome_BIOME_CERRADO, domain.BiomeCaatinga: catalogv1.Biome_BIOME_CAATINGA,
	domain.BiomePantanal: catalogv1.Biome_BIOME_PANTANAL, domain.BiomePampa: catalogv1.Biome_BIOME_PAMPA,
}

var statusToProto = map[domain.ConservationStatus]catalogv1.ConservationStatus{
	domain.StatusLC: catalogv1.ConservationStatus_CONSERVATION_STATUS_LC, domain.StatusNT: catalogv1.ConservationStatus_CONSERVATION_STATUS_NT,
	domain.StatusVU: catalogv1.ConservationStatus_CONSERVATION_STATUS_VU, domain.StatusEN: catalogv1.ConservationStatus_CONSERVATION_STATUS_EN,
	domain.StatusCR: catalogv1.ConservationStatus_CONSERVATION_STATUS_CR, domain.StatusEW: catalogv1.ConservationStatus_CONSERVATION_STATUS_EW,
	domain.StatusEX: catalogv1.ConservationStatus_CONSERVATION_STATUS_EX, domain.StatusDD: catalogv1.ConservationStatus_CONSERVATION_STATUS_DD,
}

// biomeFromProto: UNSPECIFIED vira "" (sem filtro); valor desconhecido devolve ok=false.
func biomeFromProto(b catalogv1.Biome) (string, bool) {
	if b == catalogv1.Biome_BIOME_UNSPECIFIED {
		return "", true
	}
	for d, p := range biomeToProto {
		if p == b {
			return string(d), true
		}
	}
	return "", false
}

func creditToProto(c domain.Credit) *catalogv1.Credit {
	return &catalogv1.Credit{Author: c.Author, License: c.License, Source: c.Source, SourceUrl: c.SourceURL}
}

func summaryToProto(s domain.SpeciesSummary) *catalogv1.SpeciesSummary {
	return &catalogv1.SpeciesSummary{
		Id: s.ID, ScientificName: s.ScientificName, CommonNamePt: s.CommonNamePt,
		ThumbnailKey: s.ThumbnailKey, ConservationStatus: statusToProto[s.ConservationStatus],
	}
}

func speciesToProto(s domain.Species) *catalogv1.Species {
	out := &catalogv1.Species{
		Id: s.ID, ScientificName: s.ScientificName, CommonNamePt: s.CommonNamePt, Family: s.Family,
		ConservationStatus: statusToProto[s.ConservationStatus], Description: s.Description,
		DescriptionCredit: creditToProto(s.DescriptionCredit), States: s.States, Diet: s.Diet,
	}
	if s.SizeCm != nil {
		v := toInt32(*s.SizeCm)
		out.SizeCm = &v
	}
	for _, f := range s.Facts {
		out.Facts = append(out.Facts, &catalogv1.Fact{Text: f.Text, Source: f.Source})
	}
	for _, b := range s.Biomes {
		out.Biomes = append(out.Biomes, biomeToProto[b])
	}
	for _, p := range s.Photos {
		out.Photos = append(out.Photos, &catalogv1.Photo{ThumbKey: p.ThumbKey, MediumKey: p.MediumKey, LargeKey: p.LargeKey,
			Width: toInt32(p.Width), Height: toInt32(p.Height), Credit: creditToProto(p.Credit)})
	}
	if s.Audio != nil {
		out.Audio = &catalogv1.Audio{Key: s.Audio.Key, DurationMs: toInt32(s.Audio.DurationMs), Credit: creditToProto(s.Audio.Credit)}
	}
	for _, c := range s.Clusters {
		out.Clusters = append(out.Clusters, &catalogv1.OccurrenceCluster{Lat: c.Lat, Lng: c.Lng, Count: toInt32(c.Count), Precision: c.Precision})
	}
	return out
}
