// Package grpcadapter expõe os casos de uso como um serviço gRPC.
// Ele só TRADUZ: proto → entrada do caso de uso → saída → proto.
// Nenhuma regra de negócio mora aqui.
package grpcadapter

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "github.com/velosobr/passarim-proto/gen/go/passarim/catalog/v1"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

// Interfaces pequenas (só o método Execute) facilitam testar com fakes.
type ListSpeciesUseCase interface {
	Execute(context.Context, usecase.ListSpeciesInput) (usecase.ListSpeciesOutput, error)
}
type GetSpeciesUseCase interface {
	Execute(context.Context, string) (domain.Species, error)
}
type ListFiltersUseCase interface {
	Execute(context.Context) (usecase.Filters, error)
}

// Server implementa catalogv1.CatalogServiceServer.
type Server struct {
	// Embutir o Unimplemented... é exigido pelo grpc-go: se um método novo
	// for adicionado ao .proto, o servidor continua compilando (e responde
	// "não implementado" até alguém implementar).
	catalogv1.UnimplementedCatalogServiceServer
	list    ListSpeciesUseCase
	get     GetSpeciesUseCase
	filters ListFiltersUseCase
	log     *slog.Logger
}

func NewServer(list ListSpeciesUseCase, get GetSpeciesUseCase, filters ListFiltersUseCase, log *slog.Logger) *Server {
	return &Server{list: list, get: get, filters: filters, log: log}
}

func (s *Server) ListSpecies(ctx context.Context, req *catalogv1.ListSpeciesRequest) (*catalogv1.ListSpeciesResponse, error) {
	biome, ok := biomeFromProto(req.GetBiome())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "biome: valor desconhecido")
	}
	out, err := s.list.Execute(ctx, usecase.ListSpeciesInput{
		Query: req.GetQuery(), Biome: biome, State: req.GetState(),
		PageSize: int(req.GetPageSize()), PageToken: req.GetPageToken(),
	})
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	resp := &catalogv1.ListSpeciesResponse{NextPageToken: out.NextPageToken}
	for _, sp := range out.Species {
		resp.Species = append(resp.Species, summaryToProto(sp))
	}
	return resp, nil
}

func (s *Server) GetSpecies(ctx context.Context, req *catalogv1.GetSpeciesRequest) (*catalogv1.GetSpeciesResponse, error) {
	sp, err := s.get.Execute(ctx, req.GetId())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &catalogv1.GetSpeciesResponse{Species: speciesToProto(sp)}, nil
}

func (s *Server) ListFilters(ctx context.Context, _ *catalogv1.ListFiltersRequest) (*catalogv1.ListFiltersResponse, error) {
	f, err := s.filters.Execute(ctx)
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	resp := &catalogv1.ListFiltersResponse{}
	for _, b := range f.Biomes {
		resp.Biomes = append(resp.Biomes, &catalogv1.BiomeCount{Biome: biomeToProto[b.Biome], SpeciesCount: int32(b.Count)})
	}
	for _, st := range f.States {
		resp.States = append(resp.States, &catalogv1.StateCount{State: st.State, SpeciesCount: int32(st.Count)})
	}
	return resp, nil
}

// toStatus traduz erros do domínio para códigos gRPC. Erros inesperados
// viram INTERNAL com mensagem genérica: o detalhe (que pode conter dados
// sensíveis) vai só para o log — OWASP API8.
func (s *Server) toStatus(ctx context.Context, err error) error {
	var inv *domain.InvalidArgumentError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "espécie não encontrada")
	case errors.As(err, &inv):
		return status.Error(codes.InvalidArgument, inv.Error())
	default:
		s.log.ErrorContext(ctx, "erro interno", "error", err, "request_id", RequestIDFrom(ctx))
		return status.Error(codes.Internal, "erro interno")
	}
}
