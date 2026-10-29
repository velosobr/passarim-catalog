// Testes do cliente do curador contra um servidor gRPC falso que roda na
// memória (bufconn): nenhuma porta de rede, nenhum modelo.

package curator

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	curatorv1 "github.com/velosobr/passarim-proto/gen/go/passarim/curator/v1"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// fakeServer responde cada pedido com a função reply.
type fakeServer struct {
	curatorv1.UnimplementedPhotoCuratorServiceServer
	reply func(context.Context, *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error)
}

func (f fakeServer) ClassifyPhotos(ctx context.Context, r *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error) {
	return f.reply(ctx, r)
}

// newTestClient sobe o servidor falso num bufconn (uma "rede" dentro do
// processo) e devolve o cliente ligado a ele.
func newTestClient(t *testing.T, timeout time.Duration, reply func(context.Context, *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error)) *Client {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	curatorv1.RegisterPhotoCuratorServiceServer(srv, fakeServer{reply: reply})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	c := newClient(conn, timeout)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// echo devolve SINGLE_BIRD para cada foto, na ordem.
func echo(_ context.Context, r *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error) {
	resp := &curatorv1.ClassifyPhotosResponse{ModelVersion: "m@2"}
	for _, p := range r.GetPhotos() {
		resp.Verdicts = append(resp.Verdicts, &curatorv1.PhotoVerdict{PhotoId: p.GetPhotoId(),
			Category: curatorv1.PhotoCategory_PHOTO_CATEGORY_SINGLE_BIRD, BirdCount: 1, LargestBirdArea: 0.5,
			LargestBirdCut: true, Sharpness: 0.25, Confidence: 0.75})
	}
	return resp, nil
}

var twoPhotos = []ingest.CuratorPhoto{{PhotoID: "1", Image: []byte("a")}, {PhotoID: "2", Image: []byte("b")}}

// Os campos do veredito e a versão do modelo chegam traduzidos para o domínio.
func TestClassify_MapsVerdicts(t *testing.T) {
	c := newTestClient(t, time.Second, echo)
	vs, version, err := c.ClassifyPhotos(context.Background(), twoPhotos)
	if err != nil {
		t.Fatal(err)
	}
	want := ingest.PhotoVerdict{PhotoID: "2", Category: domain.PhotoSingleBird, BirdCount: 1, LargestBirdArea: 0.5,
		LargestBirdCut: true, Sharpness: 0.25, Confidence: 0.75}
	if version != "m@2" || len(vs) != 2 || vs[1] != want {
		t.Fatalf("got %q %+v", version, vs)
	}
}

// Cada categoria do contrato vira a do domínio; um número desconhecido vira unspecified.
func TestClassify_MapsEveryCategory(t *testing.T) {
	for enum, want := range map[curatorv1.PhotoCategory]domain.PhotoCategory{
		curatorv1.PhotoCategory_PHOTO_CATEGORY_UNSPECIFIED: domain.PhotoUnspecified,
		curatorv1.PhotoCategory_PHOTO_CATEGORY_SINGLE_BIRD: domain.PhotoSingleBird,
		curatorv1.PhotoCategory_PHOTO_CATEGORY_FLOCK:       domain.PhotoFlock,
		curatorv1.PhotoCategory_PHOTO_CATEGORY_NEST:        domain.PhotoNest,
		curatorv1.PhotoCategory_PHOTO_CATEGORY_EGGS:        domain.PhotoEggs,
		curatorv1.PhotoCategory_PHOTO_CATEGORY_FEATHER:     domain.PhotoFeather,
		curatorv1.PhotoCategory_PHOTO_CATEGORY_DEAD_BIRD:   domain.PhotoDeadBird,
		curatorv1.PhotoCategory_PHOTO_CATEGORY_NO_BIRD:     domain.PhotoNoBird,
		curatorv1.PhotoCategory_PHOTO_CATEGORY_OTHER:       domain.PhotoOther,
		curatorv1.PhotoCategory(99):                        domain.PhotoUnspecified,
	} {
		if got := category(enum); got != want {
			t.Errorf("category(%v) = %q, esperava %q", enum, got, want)
		}
	}
}

// UNAVAILABLE (curador subindo) vira ErrCuratorUnavailable.
func TestClassify_UnavailableIsUnavailable(t *testing.T) {
	c := newTestClient(t, time.Second, func(context.Context, *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error) {
		return nil, status.Error(codes.Unavailable, "subindo")
	})
	if _, _, err := c.ClassifyPhotos(context.Background(), twoPhotos); !errors.Is(err, ingest.ErrCuratorUnavailable) {
		t.Fatalf("esperava ErrCuratorUnavailable, veio %v", err)
	}
}

// Sem resposta dentro do prazo do cliente: DEADLINE_EXCEEDED, também indisponível.
func TestClassify_TimeoutIsUnavailable(t *testing.T) {
	c := newTestClient(t, 50*time.Millisecond, func(ctx context.Context, r *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if _, _, err := c.ClassifyPhotos(context.Background(), twoPhotos); !errors.Is(err, ingest.ErrCuratorUnavailable) {
		t.Fatalf("esperava ErrCuratorUnavailable, veio %v", err)
	}
}

// O contexto de quem chama cancelado (SIGTERM, fim do prazo do job): também
// indisponível, para o job ser adiado e não gastar tentativa.
func TestClassify_CanceledIsUnavailable(t *testing.T) {
	c := newTestClient(t, time.Second, echo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.ClassifyPhotos(ctx, twoPhotos); !errors.Is(err, ingest.ErrCuratorUnavailable) {
		t.Fatalf("esperava ErrCuratorUnavailable, veio %v", err)
	}
}

// INVALID_ARGUMENT é bug nosso: erro comum, que gasta tentativa.
func TestClassify_InvalidArgumentIsPlainError(t *testing.T) {
	c := newTestClient(t, time.Second, func(context.Context, *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error) {
		return nil, status.Error(codes.InvalidArgument, "lote vazio")
	})
	_, _, err := c.ClassifyPhotos(context.Background(), twoPhotos)
	if err == nil || errors.Is(err, ingest.ErrCuratorUnavailable) {
		t.Fatalf("esperava erro comum, veio %v", err)
	}
}

// Resposta com outro número de vereditos ou em outra ordem é recusada:
// aceitar daria a categoria de uma foto a outra.
func TestClassify_RejectsMismatchedReply(t *testing.T) {
	replies := map[string]func(context.Context, *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error){
		"um a menos": func(ctx context.Context, r *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error) {
			resp, _ := echo(ctx, r)
			resp.Verdicts = resp.Verdicts[:1]
			return resp, nil
		},
		"fora de ordem": func(ctx context.Context, r *curatorv1.ClassifyPhotosRequest) (*curatorv1.ClassifyPhotosResponse, error) {
			resp, _ := echo(ctx, r)
			resp.Verdicts[0], resp.Verdicts[1] = resp.Verdicts[1], resp.Verdicts[0]
			return resp, nil
		},
	}
	for name, reply := range replies {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, time.Second, reply)
			_, _, err := c.ClassifyPhotos(context.Background(), twoPhotos)
			if err == nil || errors.Is(err, ingest.ErrCuratorUnavailable) {
				t.Fatalf("esperava erro de contrato, veio %v", err)
			}
		})
	}
}

// Mais de 8 fotos nem sai do worker: é bug do caso de uso.
func TestClassify_RejectsBigBatch(t *testing.T) {
	c := newTestClient(t, time.Second, echo)
	if _, _, err := c.ClassifyPhotos(context.Background(), make([]ingest.CuratorPhoto, 9)); err == nil {
		t.Fatal("lote de 9 deveria ser recusado")
	}
}
