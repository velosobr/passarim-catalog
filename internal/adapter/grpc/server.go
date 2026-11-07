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
	Execute(context.Context, usecase.ListFiltersInput) (usecase.Filters, error)
}
type FeaturedUseCase interface {
	Execute(context.Context, string) (usecase.FeaturedOutput, error)
}

// Server implementa catalogv1.CatalogServiceServer.
type Server struct {
	// Embutir o Unimplemented... é exigido pelo grpc-go: se um método novo
	// for adicionado ao .proto, o servidor continua compilando (e responde
	// "não implementado" até alguém implementar).
	catalogv1.UnimplementedCatalogServiceServer
	list     ListSpeciesUseCase
	get      GetSpeciesUseCase
	filters  ListFiltersUseCase
	featured FeaturedUseCase
	log      *slog.Logger
}

// NewServer liga o servidor aos quatro casos de uso.
func NewServer(list ListSpeciesUseCase, get GetSpeciesUseCase, filters ListFiltersUseCase, featured FeaturedUseCase, log *slog.Logger) *Server {
	return &Server{list: list, get: get, filters: filters, featured: featured, log: log}
}

// ListSpecies converte o pedido gRPC na entrada do caso de uso e a saída de
// volta. Um bioma com número que o catalog não conhece é recusado na hora.
func (s *Server) ListSpecies(ctx context.Context, req *catalogv1.ListSpeciesRequest) (*catalogv1.ListSpeciesResponse, error) {
	biome, ok := biomeFromProto(req.GetBiome())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "biome: valor desconhecido")
	}
	primary, ok := biomeFromProto(req.GetPrimaryBiome())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "primary_biome: valor desconhecido")
	}
	out, err := s.list.Execute(ctx, usecase.ListSpeciesInput{
		Query: req.GetQuery(), Biome: biome, State: req.GetState(),
		KidsOnly: req.GetKidsOnly(), PrimaryBiome: primary,
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

// GetSpecies busca uma espécie pelo id.
func (s *Server) GetSpecies(ctx context.Context, req *catalogv1.GetSpeciesRequest) (*catalogv1.GetSpeciesResponse, error) {
	sp, err := s.get.Execute(ctx, req.GetId())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &catalogv1.GetSpeciesResponse{Species: speciesToProto(sp)}, nil
}

// ListFilters devolve as contagens por bioma, por UF e por bioma principal.
// kids_only = contar só as espécies com conteúdo Kids.
func (s *Server) ListFilters(ctx context.Context, req *catalogv1.ListFiltersRequest) (*catalogv1.ListFiltersResponse, error) {
	f, err := s.filters.Execute(ctx, usecase.ListFiltersInput{KidsOnly: req.GetKidsOnly()})
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	resp := &catalogv1.ListFiltersResponse{}
	for _, b := range f.Biomes {
		resp.Biomes = append(resp.Biomes, &catalogv1.BiomeCount{Biome: biomeToProto[b.Biome], SpeciesCount: toInt32(b.Count)})
	}
	for _, st := range f.States {
		resp.States = append(resp.States, &catalogv1.StateCount{State: st.State, SpeciesCount: toInt32(st.Count)})
	}
	for _, b := range f.PrimaryBiomes {
		resp.PrimaryBiomeCounts = append(resp.PrimaryBiomeCounts, &catalogv1.BiomeCount{Biome: biomeToProto[b.Biome], SpeciesCount: toInt32(b.Count)})
	}
	return resp, nil
}

// GetFeaturedSpecies devolve a "Ave do dia" da data pedida (AAAA-MM-DD, no
// fuso de São Paulo, calculada pelo BFF). O canto vai junto porque o card
// adulto tem botão de canto; sem canto, o campo fica ausente.
func (s *Server) GetFeaturedSpecies(ctx context.Context, req *catalogv1.GetFeaturedSpeciesRequest) (*catalogv1.GetFeaturedSpeciesResponse, error) {
	out, err := s.featured.Execute(ctx, req.GetDate())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	resp := &catalogv1.GetFeaturedSpeciesResponse{Species: summaryToProto(out.Species)}
	if out.Audio != nil {
		resp.Audio = audioToProto(*out.Audio)
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
		return status.Error(codes.NotFound, "não encontrado")
	case errors.As(err, &inv):
		return status.Error(codes.InvalidArgument, inv.Error())
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		// O BFF chama com timeout de 2s: cliente cancelando ou prazo
		// expirando é operação normal, não falha do catalog — por isso
		// vira DEADLINE_EXCEEDED/CANCELLED (não INTERNAL) e é logado como
		// WARN, sem poluir métricas e alertas de erro.
		st := status.FromContextError(err)
		s.log.WarnContext(ctx, "requisição cancelada ou expirou", "error", err, "code", st.Code(), "request_id", RequestIDFrom(ctx))
		return st.Err()
	default:
		s.log.ErrorContext(ctx, "erro interno", "error", err, "request_id", RequestIDFrom(ctx))
		return status.Error(codes.Internal, "erro interno")
	}
}
