package grpcadapter_test

import (
	"context"
	"errors"
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

type fakeFilters struct{ f usecase.Filters }

func (f fakeFilters) Execute(context.Context) (usecase.Filters, error) { return f.f, nil }

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

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestListSpecies_MapsRequestAndResponse(t *testing.T) {
	list := &fakeList{out: usecase.ListSpeciesOutput{
		Species:       []domain.SpeciesSummary{{ID: "a", CommonNamePt: "Ave", ScientificName: "A a", ThumbnailKey: "k", ConservationStatus: domain.StatusVU}},
		NextPageToken: "tok",
	}}
	c := dial(t, grpcadapter.NewServer(list, fakeGet{}, fakeFilters{}, quiet))
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

func TestListSpecies_UnknownBiomeEnumIsInvalidArgument(t *testing.T) {
	c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{}, fakeFilters{}, quiet))
	_, err := c.ListSpecies(context.Background(), &catalogv1.ListSpeciesRequest{Biome: catalogv1.Biome(99)})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v", status.Code(err))
	}
}

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
		c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{err: tc.err}, fakeFilters{}, quiet))
		_, err := c.GetSpecies(context.Background(), &catalogv1.GetSpeciesRequest{Id: "x"})
		if status.Code(err) != tc.code {
			t.Errorf("%v → %v, want %v", tc.err, status.Code(err), tc.code)
		}
		if tc.code == codes.Internal && status.Convert(err).Message() != "erro interno" {
			t.Errorf("erro interno vazou detalhes: %q", status.Convert(err).Message())
		}
	}
}

func TestGetSpecies_MapsOptionalsAndLists(t *testing.T) {
	size := 25
	s := domain.Species{
		ID: "t", ScientificName: "T r", CommonNamePt: "Sabiá", Family: "Turdidae", SizeCm: &size,
		Biomes: []domain.Biome{domain.BiomeCerrado}, States: []string{"SP"},
		Facts:    []domain.Fact{{Text: "f", Source: "s"}},
		Photos:   []domain.Photo{{ThumbKey: "t.webp", Width: 10, Height: 5, Credit: domain.Credit{Author: "A", License: "CC-BY"}}},
		Clusters: []domain.OccurrenceCluster{{Lat: 1, Lng: 2, Count: 3, Precision: 0.5}},
	}
	c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{s: s}, fakeFilters{}, quiet))
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

func TestListFilters(t *testing.T) {
	f := usecase.Filters{Biomes: []usecase.BiomeCount{{Biome: domain.BiomePampa, Count: 2}}, States: []usecase.StateCount{{State: "RS", Count: 2}}}
	c := dial(t, grpcadapter.NewServer(&fakeList{}, fakeGet{}, fakeFilters{f: f}, quiet))
	resp, err := c.ListFilters(context.Background(), &catalogv1.ListFiltersRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetBiomes()[0].GetBiome() != catalogv1.Biome_BIOME_PAMPA || resp.GetStates()[0].GetSpeciesCount() != 2 {
		t.Fatalf("filtros: %v", resp)
	}
}
