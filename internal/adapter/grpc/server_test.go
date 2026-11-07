// Testes do servidor gRPC com casos de uso falsos e um cliente gRPC real,
// numa conexão em memória.

package grpcadapter_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	catalogv1 "github.com/velosobr/passarim-proto/gen/go/passarim/catalog/v1"

	grpcadapter "github.com/velosobr/passarim-catalog/internal/adapter/grpc"
	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase"
)

// Os três casos de uso falsos guardam a entrada e devolvem o configurado.
type fakeList struct {
	in  usecase.ListSpeciesInput
	out usecase.ListSpeciesOutput
	err error
}

func (f *fakeList) Execute(_ context.Context, in usecase.ListSpeciesInput) (usecase.ListSpeciesOutput, error) {
	f.in = in
	return f.out, f.err
}

type fakeGet struct {
	s   domain.Species
	err error
}

func (f fakeGet) Execute(context.Context, string) (domain.Species, error) { return f.s, f.err }

type fakeFilters struct {
	f  usecase.Filters
	in usecase.ListFiltersInput
}

func (f *fakeFilters) Execute(_ context.Context, in usecase.ListFiltersInput) (usecase.Filters, error) {
	f.in = in
	return f.f, nil
}

// dial sobe o servidor gRPC em memória (bufconn): sem rede, rápido e real.
func dial(t *testing.T, srv catalogv1.CatalogServiceServer) catalogv1.CatalogServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	catalogv1.RegisterCatalogServiceServer(s, srv)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return catalogv1.NewCatalogServiceClient(conn)
}

// quiet é um logger que descarta tudo.
var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// Os campos do pedido chegam ao caso de uso, e a resposta volta ao proto.
func TestListSpecies_MapsRequestAndResponse(t *testing.T) {
	list := &fakeList{out: usecase.ListSpeciesOutput{
		Species:       []domain.SpeciesSummary{{ID: "a", CommonNamePt: "Ave", ScientificName: "A a", ThumbnailKey: "k", ConservationStatus: domain.StatusVU}},
		NextPageToken: "tok",
	}}
	c := dial(t, grpcadapter.NewServer(list, fakeGet{}, &fakeFilters{}, quiet))
	resp, err := c.ListSpecies(context.Background(), &catalogv1.ListSpeciesRequest{
		Query: "sabia", Biome: catalogv1.Biome_BIOME_PANTANAL, State: "MS", PageSize: 10, PageToken: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if list.in != (usecase.ListSpeciesInput{Query: "sabia", Biome: "pantanal", State: "MS", PageSize: 10, PageToken: "x"}) {
		t.Fatalf("entrada mapeada errado: %+v", list.in)
	}
	got := resp.GetSpecies()[0]
	if got.GetId() != "a" || got.GetConservationStatus() != catalogv1.ConservationStatus_CONSERVATION_STATUS_VU || resp.GetNextPageToken() != "tok" {
		t.Fatalf("resposta mapeada errado: %v", resp)
	}
}

// Um número de bioma que não existe no enum vira INVALID_ARGUMENT.
func TestListSpecies_UnknownBiomeEnumIsInvalidArgument(t *testing.T) {
	c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{}, &fakeFilters{}, quiet))
	_, err := c.ListSpecies(context.Background(), &catalogv1.ListSpeciesRequest{Biome: catalogv1.Biome(99)})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v", status.Code(err))
	}
}

// Cada erro do domínio vira o código gRPC certo, e um erro inesperado vira
// INTERNAL sem vazar a mensagem original.
func TestErrorsMapToGRPCCodes(t *testing.T) {
	cases := []struct {
		err  error
		code codes.Code
	}{
		{domain.ErrNotFound, codes.NotFound},
		{&domain.InvalidArgumentError{Field: "id", Reason: "x"}, codes.InvalidArgument},
		{errors.New("senha do banco: hunter2"), codes.Internal},
	}
	for _, tc := range cases {
		c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{err: tc.err}, &fakeFilters{}, quiet))
		_, err := c.GetSpecies(context.Background(), &catalogv1.GetSpeciesRequest{Id: "x"})
		if status.Code(err) != tc.code {
			t.Errorf("%v → %v, want %v", tc.err, status.Code(err), tc.code)
		}
		if tc.code == codes.Internal && status.Convert(err).Message() != "erro interno" {
			t.Errorf("erro interno vazou detalhes: %q", status.Convert(err).Message())
		}
	}
}

// Ctx cancelado/expirado (ex.: timeout
// de 2s do BFF) não pode virar INTERNAL — isso polui as métricas de erro.
// DeadlineExceeded e Canceled têm códigos gRPC próprios.
func TestErrorsMapToGRPCCodes_ContextCancellationAndDeadline(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code codes.Code
	}{
		{"deadline exceeded (encapsulado)", fmt.Errorf("listar espécies: %w", context.DeadlineExceeded), codes.DeadlineExceeded},
		{"canceled (encapsulado)", fmt.Errorf("listar espécies: %w", context.Canceled), codes.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{err: tc.err}, &fakeFilters{}, quiet))
			_, err := c.GetSpecies(context.Background(), &catalogv1.GetSpeciesRequest{Id: "x"})
			if status.Code(err) != tc.code {
				t.Fatalf("code = %v, want %v", status.Code(err), tc.code)
			}
		})
	}
}

// levelCapture é um slog.Handler mínimo que só guarda o nível de cada log,
// para provar que cancelamento/deadline NÃO são logados como ERROR.
type levelCapture struct{ levels []slog.Level }

// Os quatro métodos abaixo são a interface slog.Handler.
func (c *levelCapture) Enabled(context.Context, slog.Level) bool { return true }
func (c *levelCapture) Handle(_ context.Context, r slog.Record) error {
	c.levels = append(c.levels, r.Level)
	return nil
}
func (c *levelCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *levelCapture) WithGroup(string) slog.Handler      { return c }

// Cancelamento e prazo esgotado são logados abaixo de ERROR, para não
// disparar alertas de erro.
func TestErrorsMapToGRPCCodes_ContextCancellationLogsBelowError(t *testing.T) {
	cap := &levelCapture{}
	log := slog.New(cap)
	c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{err: fmt.Errorf("x: %w", context.DeadlineExceeded)}, &fakeFilters{}, log))
	_, _ = c.GetSpecies(context.Background(), &catalogv1.GetSpeciesRequest{Id: "x"})
	if len(cap.levels) != 1 || cap.levels[0] >= slog.LevelError {
		t.Fatalf("esperava log abaixo de ERROR para deadline/cancelamento, veio %v", cap.levels)
	}
}

// Campos opcionais e listas da espécie chegam ao proto.
func TestGetSpecies_MapsOptionalsAndLists(t *testing.T) {
	size := 25
	s := domain.Species{
		ID: "t", ScientificName: "T r", CommonNamePt: "Sabiá", Family: "Turdidae", SizeCm: &size,
		Biomes: []domain.Biome{domain.BiomeCerrado}, States: []string{"SP"},
		Facts:    []domain.Fact{{Text: "f", Source: "s"}},
		Photos:   []domain.Photo{{ThumbKey: "t.webp", Width: 10, Height: 5, Credit: domain.Credit{Author: "A", License: "CC-BY"}}},
		Clusters: []domain.OccurrenceCluster{{Lat: 1, Lng: 2, Count: 3, Precision: 0.5}},
	}
	c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{s: s}, &fakeFilters{}, quiet))
	resp, err := c.GetSpecies(context.Background(), &catalogv1.GetSpeciesRequest{Id: "t"})
	if err != nil {
		t.Fatal(err)
	}
	got := resp.GetSpecies()
	if got.SizeCm == nil || got.GetSizeCm() != 25 || got.Diet != nil || got.Audio != nil {
		t.Fatalf("opcionais errados: size=%v diet=%v audio=%v", got.SizeCm, got.Diet, got.Audio)
	}
	if got.GetBiomes()[0] != catalogv1.Biome_BIOME_CERRADO || got.GetPhotos()[0].GetCredit().GetAuthor() != "A" || got.GetClusters()[0].GetPrecision() != 0.5 {
		t.Fatalf("listas erradas: %v", got)
	}
}

// Os filtros chegam ao proto com as contagens.
func TestListFilters(t *testing.T) {
	f := usecase.Filters{Biomes: []usecase.BiomeCount{{Biome: domain.BiomePampa, Count: 2}}, States: []usecase.StateCount{{State: "RS", Count: 2}},
		PrimaryBiomes: []usecase.BiomeCount{{Biome: domain.BiomeCerrado, Count: 4}}}
	filters := &fakeFilters{f: f}
	c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{}, filters, quiet))
	resp, err := c.ListFilters(context.Background(), &catalogv1.ListFiltersRequest{KidsOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetBiomes()[0].GetBiome() != catalogv1.Biome_BIOME_PAMPA || resp.GetStates()[0].GetSpeciesCount() != 2 {
		t.Fatalf("filtros: %v", resp)
	}
	if !filters.in.KidsOnly || resp.GetPrimaryBiomeCounts()[0].GetBiome() != catalogv1.Biome_BIOME_CERRADO || resp.GetPrimaryBiomeCounts()[0].GetSpeciesCount() != 4 {
		t.Fatalf("kids_only=%v principais=%v", filters.in.KidsOnly, resp.GetPrimaryBiomeCounts())
	}
}

// O pedido da lista leva kids_only e o bioma principal ao caso de uso, e
// um bioma principal desconhecido é recusado.
func TestListSpecies_KidsFilters(t *testing.T) {
	list := &fakeList{}
	c := dial(t, grpcadapter.NewServer(list, fakeGet{}, &fakeFilters{}, quiet))
	_, err := c.ListSpecies(context.Background(), &catalogv1.ListSpeciesRequest{KidsOnly: true, PrimaryBiome: catalogv1.Biome_BIOME_PAMPA})
	if err != nil {
		t.Fatal(err)
	}
	if !list.in.KidsOnly || list.in.PrimaryBiome != "pampa" {
		t.Fatalf("entrada: %+v", list.in)
	}
	_, err = c.ListSpecies(context.Background(), &catalogv1.ListSpeciesRequest{PrimaryBiome: catalogv1.Biome(99)})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bioma principal 99 deveria ser INVALID_ARGUMENT, veio %v", err)
	}
}

// O detalhe leva o conteúdo Kids e o bioma principal; sem Kids, o campo fica ausente.
func TestGetSpecies_Kids(t *testing.T) {
	s := domain.Species{ID: "t", PrimaryBiome: domain.BiomeCerrado,
		Kids: &domain.KidsContent{Description: "D", DidYouKnow: "V", Facts: []domain.KidsFact{{Text: "F", SourceFact: 1}}}}
	c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{s: s}, &fakeFilters{}, quiet))
	resp, err := c.GetSpecies(context.Background(), &catalogv1.GetSpeciesRequest{Id: "t"})
	if err != nil {
		t.Fatal(err)
	}
	k := resp.GetSpecies().GetKids()
	if resp.GetSpecies().GetPrimaryBiome() != catalogv1.Biome_BIOME_CERRADO || k.GetDescription() != "D" || k.GetDidYouKnow() != "V" ||
		k.GetFacts()[0].GetSourceFact() != 1 {
		t.Fatalf("kids: %v", resp.GetSpecies())
	}
	s.Kids, s.PrimaryBiome = nil, ""
	c = dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{s: s}, &fakeFilters{}, quiet))
	resp, _ = c.GetSpecies(context.Background(), &catalogv1.GetSpeciesRequest{Id: "t"})
	if resp.GetSpecies().Kids != nil || resp.GetSpecies().GetPrimaryBiome() != catalogv1.Biome_BIOME_UNSPECIFIED {
		t.Fatalf("sem Kids, o campo deveria ficar ausente: %v", resp.GetSpecies())
	}
}

// O canto chega ao proto com os picos.
func TestGetSpecies_AudioPeaks(t *testing.T) {
	s := domain.Species{ID: "t", Audio: &domain.Audio{Key: "a.aac", Peaks: []float32{0, 1}}}
	c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{s: s}, &fakeFilters{}, quiet))
	resp, err := c.GetSpecies(context.Background(), &catalogv1.GetSpeciesRequest{Id: "t"})
	if err != nil || len(resp.GetSpecies().GetAudio().GetPeaks()) != 2 || resp.GetSpecies().GetAudio().GetPeaks()[1] != 1 {
		t.Fatalf("err=%v audio=%v", err, resp.GetSpecies().GetAudio())
	}
}
