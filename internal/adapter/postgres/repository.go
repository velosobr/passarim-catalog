package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/velosobr/passarim-catalog/internal/adapter/postgres/sqlcgen"
	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

// toInt32 converte um int para int32 saturando nos limites do int32 em vez
// de dar overflow silencioso (gosec G115). Limit e SizeCm já são pequenos e
// validados (ver usecase.buildFilter e domain.Species.Validate), então a
// saturação nunca deve ocorrer na prática.
func toInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// Repository implementa usecase.SpeciesRepository com PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// Garante, em tempo de compilação, que Repository cumpre a interface.
var _ usecase.SpeciesRepository = (*Repository)(nil)

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: sqlcgen.New(pool)}
}

// escapeLike faz "%" e "_" digitados pelo usuário valerem como texto,
// e não como curingas do LIKE ("%" = qualquer coisa). A barra também é
// escapada porque é o caractere de escape que declaramos no SQL.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func (r *Repository) ListSpecies(ctx context.Context, f usecase.ListFilter) ([]domain.SpeciesSummary, error) {
	params := sqlcgen.ListSpeciesParams{
		Query: text(escapeLike(f.Query)),
		State: text(f.State),
		Lim:   toInt32(f.Limit),
	}
	if f.Biome != "" {
		params.Biome = sqlcgen.NullBiome{Biome: sqlcgen.Biome(f.Biome), Valid: true}
	}
	if f.After != nil {
		params.AfterSort = text(f.After.SortName)
		params.AfterID = text(f.After.ID)
		params.AfterSort.Valid = true // SortName pode ser "" legitimamente
	}
	rows, err := r.q.ListSpecies(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SpeciesSummary, len(rows))
	for i, row := range rows {
		out[i] = domain.SpeciesSummary{
			ID: row.ID, ScientificName: row.ScientificName, CommonNamePt: row.CommonNamePt,
			ThumbnailKey: row.ThumbnailKey, ConservationStatus: domain.ConservationStatus(row.ConservationStatus),
		}
	}
	return out, nil
}

func (r *Repository) GetSpecies(ctx context.Context, id string) (domain.Species, error) {
	row, err := r.q.GetSpecies(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Species{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Species{}, err
	}
	s := domain.Species{
		ID: row.ID, ScientificName: row.ScientificName, CommonNamePt: row.CommonNamePt, Family: row.Family,
		ConservationStatus: domain.ConservationStatus(row.ConservationStatus), Description: row.Description,
		DescriptionCredit: domain.Credit{Author: row.DescriptionAuthor, License: row.DescriptionLicense,
			Source: row.DescriptionSource, SourceURL: row.DescriptionSourceUrl},
	}
	if row.SizeCm.Valid {
		v := int(row.SizeCm.Int32)
		s.SizeCm = &v
	}
	if row.Diet.Valid {
		v := row.Diet.String
		s.Diet = &v
	}

	facts, err := r.q.ListFacts(ctx, id)
	if err != nil {
		return s, err
	}
	for _, f := range facts {
		s.Facts = append(s.Facts, domain.Fact{Text: f.Text, Source: f.Source})
	}
	biomes, err := r.q.ListBiomes(ctx, id)
	if err != nil {
		return s, err
	}
	for _, b := range biomes {
		s.Biomes = append(s.Biomes, domain.Biome(b))
	}
	if s.States, err = r.q.ListStates(ctx, id); err != nil {
		return s, err
	}
	media, err := r.q.ListMedia(ctx, id)
	if err != nil {
		return s, err
	}
	for _, m := range media {
		credit := domain.Credit{Author: m.Author, License: m.License, Source: m.Source, SourceURL: m.SourceUrl}
		switch m.Kind {
		case "photo":
			s.Photos = append(s.Photos, domain.Photo{ThumbKey: m.ThumbKey, MediumKey: m.MediumKey, LargeKey: m.LargeKey,
				Width: int(m.Width), Height: int(m.Height), Credit: credit})
		case "audio":
			if s.Audio == nil { // a primeira gravação (menor position) é a escolhida
				s.Audio = &domain.Audio{Key: m.AudioKey, DurationMs: int(m.DurationMs), Credit: credit}
			}
		}
	}
	clusters, err := r.q.ListClusters(ctx, id)
	if err != nil {
		return s, err
	}
	for _, c := range clusters {
		s.Clusters = append(s.Clusters, domain.OccurrenceCluster{Lat: c.Lat, Lng: c.Lng, Count: int(c.Count), Precision: c.Precision})
	}
	return s, nil
}

func (r *Repository) CountByBiome(ctx context.Context) (map[domain.Biome]int, error) {
	rows, err := r.q.CountByBiome(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.Biome]int, len(rows))
	for _, row := range rows {
		out[domain.Biome(row.Biome)] = int(row.Total)
	}
	return out, nil
}

func (r *Repository) CountByState(ctx context.Context) (map[string]int, error) {
	rows, err := r.q.CountByState(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.Uf] = int(row.Total)
	}
	return out, nil
}

// UpsertCurated grava (ou atualiza) os campos CURADOS de uma espécie numa
// transação: ou tudo é gravado, ou nada. Fotos, cantos e ocorrências
// pertencem ao worker e NÃO são tocados aqui.
func (r *Repository) UpsertCurated(ctx context.Context, s domain.Species) error {
	if err := s.Validate(); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // após Commit, Rollback não faz nada
	q := r.q.WithTx(tx)

	params := sqlcgen.UpsertSpeciesParams{
		ID: s.ID, ScientificName: s.ScientificName, CommonNamePt: s.CommonNamePt, Family: s.Family,
		ConservationStatus: string(s.ConservationStatus), Description: s.Description,
		DescriptionAuthor: s.DescriptionCredit.Author, DescriptionLicense: s.DescriptionCredit.License,
		DescriptionSource: s.DescriptionCredit.Source, DescriptionSourceUrl: s.DescriptionCredit.SourceURL,
		SortName:   domain.NormalizeForSearch(s.CommonNamePt),
		SearchText: domain.NormalizeForSearch(s.CommonNamePt + " " + s.ScientificName),
	}
	if s.SizeCm != nil {
		params.SizeCm = pgtype.Int4{Int32: toInt32(*s.SizeCm), Valid: true}
	}
	if s.Diet != nil {
		params.Diet = pgtype.Text{String: *s.Diet, Valid: true}
	}
	if err := q.UpsertSpecies(ctx, params); err != nil {
		return fmt.Errorf("upsert %s: %w", s.ID, err)
	}

	// Listas: apagar e reinserir é o jeito mais simples de deixar o banco
	// igual ao YAML (inclusive removendo itens que saíram do arquivo).
	if err := q.DeleteFacts(ctx, s.ID); err != nil {
		return err
	}
	for i, f := range s.Facts {
		if err := q.InsertFact(ctx, sqlcgen.InsertFactParams{SpeciesID: s.ID, Position: int32(i), Text: f.Text, Source: f.Source}); err != nil {
			return err
		}
	}
	if err := q.DeleteBiomes(ctx, s.ID); err != nil {
		return err
	}
	for _, b := range s.Biomes {
		if err := q.InsertBiome(ctx, sqlcgen.InsertBiomeParams{SpeciesID: s.ID, Biome: sqlcgen.Biome(b)}); err != nil {
			return err
		}
	}
	if err := q.DeleteStates(ctx, s.ID); err != nil {
		return err
	}
	for _, uf := range s.States {
		if err := q.InsertState(ctx, sqlcgen.InsertStateParams{SpeciesID: s.ID, Uf: uf}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
