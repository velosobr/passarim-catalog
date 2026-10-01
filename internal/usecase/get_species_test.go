package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

func TestGetSpecies(t *testing.T) {
	uc := usecase.GetSpecies{Repo: &fakeRepo{species: birds(3)}}
	want := birds(3)[1]
	got, err := uc.Execute(context.Background(), "  "+want.ID+" ")
	if err != nil || got.ID != want.ID {
		t.Fatalf("got %v, %v", got.ID, err)
	}
	if _, err := uc.Execute(context.Background(), "nao-existe"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("esperava ErrNotFound, veio %v", err)
	}
	_, err = uc.Execute(context.Background(), "   ")
	assertInvalid(t, err, "id")
}
