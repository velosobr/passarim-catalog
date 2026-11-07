// Caso de uso da "Ave do dia" (frente B): valida a data, devolve a ave já
// gravada para o dia ou escolhe uma pelo rodízio do domínio e grava.

package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata" // embute o banco de fusos: a imagem do catalog pode não ter o do sistema

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// saoPaulo é o fuso em que o "dia" da Ave do dia é contado. LoadLocation lê
// o banco de fusos horários (com as regras de horário de verão, se voltarem);
// o import de time/tzdata garante que ele exista em qualquer imagem.
var saoPaulo = mustLoadLocation("America/Sao_Paulo")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err) // com time/tzdata embutido, só acontece se o nome estiver errado
	}
	return loc
}

// FeaturedOutput é a ave do dia: o resumo do card e o canto (nil se a ave
// perdeu o canto depois de ser escolhida).
type FeaturedOutput struct {
	Species domain.SpeciesSummary
	Audio   *domain.Audio
}

// GetFeatured escolhe a "Ave do dia". Now é o relógio, injetado para os
// testes pararem o tempo onde quiserem (em produção, time.Now).
type GetFeatured struct {
	Repo FeaturedRepository
	Now  func() time.Time
}

// Execute aceita só ontem, hoje e amanhã no fuso de São Paulo. A folga cobre
// relógios um pouco diferentes entre o BFF (que calcula a data) e o catalog
// na virada do dia; recusar o resto impede gravar dias arbitrários.
//
// Se o dia já tem ave, devolve a mesma (é o que deixa a ave estável o dia
// todo, mesmo que ela deixe de ser elegível). Senão, escolhe pelo rodízio e
// grava. Duas réplicas podem escolher ao mesmo tempo: a gravação ignora a
// segunda (ON CONFLICT DO NOTHING) e as duas releem a linha que ficou.
func (u GetFeatured) Execute(ctx context.Context, date string) (FeaturedOutput, error) {
	day, err := time.ParseInLocation(time.DateOnly, date, saoPaulo)
	if err != nil || day.Format(time.DateOnly) != date {
		return FeaturedOutput{}, &domain.InvalidArgumentError{Field: "date", Reason: "use o formato AAAA-MM-DD"}
	}
	now := u.Now().In(saoPaulo)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, saoPaulo)
	if day.Before(today.AddDate(0, 0, -1)) || day.After(today.AddDate(0, 0, 1)) {
		return FeaturedOutput{}, &domain.InvalidArgumentError{Field: "date", Reason: "só ontem, hoje ou amanhã no fuso de São Paulo"}
	}

	id, found, err := u.Repo.FeaturedOn(ctx, date)
	if err != nil {
		return FeaturedOutput{}, fmt.Errorf("ler a ave do dia: %w", err)
	}
	if !found {
		if id, err = u.pick(ctx, date); err != nil {
			return FeaturedOutput{}, err
		}
	}
	s, err := u.Repo.GetSpecies(ctx, id)
	if err != nil {
		return FeaturedOutput{}, err
	}
	out := FeaturedOutput{Species: domain.SpeciesSummary{ID: s.ID, ScientificName: s.ScientificName, CommonNamePt: s.CommonNamePt,
		ConservationStatus: s.ConservationStatus, PrimaryBiome: s.PrimaryBiome}, Audio: s.Audio}
	if len(s.Photos) > 0 {
		out.Species.ThumbnailKey = s.Photos[0].ThumbKey // a foto 0 é a principal
	}
	return out, nil
}

// pick roda o rodízio, grava a escolha e relê a linha do dia (que pode ser
// a de outra réplica que gravou primeiro).
func (u GetFeatured) pick(ctx context.Context, date string) (string, error) {
	eligible, err := u.Repo.FeaturedEligible(ctx)
	if err != nil {
		return "", fmt.Errorf("listar elegíveis: %w", err)
	}
	rows, err := u.Repo.FeaturedCycle(ctx, date)
	if err != nil {
		return "", fmt.Errorf("ler o ciclo: %w", err)
	}
	id, cycle, ok := domain.PickFeatured(date, eligible, rows)
	if !ok {
		return "", domain.ErrNotFound
	}
	if err := u.Repo.SaveFeatured(ctx, domain.FeaturedDay{Day: date, SpeciesID: id, Cycle: cycle}); err != nil {
		return "", fmt.Errorf("gravar a ave do dia: %w", err)
	}
	saved, found, err := u.Repo.FeaturedOn(ctx, date)
	if err != nil {
		return "", fmt.Errorf("reler a ave do dia: %w", err)
	}
	if !found {
		return "", errors.New("a ave do dia sumiu logo depois de gravada")
	}
	return saved, nil
}
