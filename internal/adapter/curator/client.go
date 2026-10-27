// Package curator é o cliente gRPC do passarim-photo-curator, o serviço em
// Python que diz o que cada foto mostra (ave sozinha, bando, ninho...).
//
// Quem usa: o caso de uso IngestPhotos do worker, pela porta
// ingest.PhotoCurator. Só o worker fala com o curador; a API, o BFF e o app
// nem sabem que ele existe.
//
// gRPC é um jeito de chamar uma função em outro processo pela rede: o
// contrato (curator.proto, no passarim-proto) define as mensagens, e o
// código gerado faz a conversa. Aqui só traduzimos os tipos gerados para os
// do domínio e os erros do gRPC para os que o caso de uso entende.
package curator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	curatorv1 "github.com/velosobr/passarim-proto/gen/go/passarim/curator/v1"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// Client fala com o curador. timeout é o prazo de cada chamada (o
// "deadline" do gRPC: passado esse tempo, a chamada desiste com
// DEADLINE_EXCEEDED).
type Client struct {
	conn    *grpc.ClientConn
	api     curatorv1.PhotoCuratorServiceClient
	timeout time.Duration
}

// Conferência em tempo de compilação: Client implementa ingest.PhotoCurator.
var _ ingest.PhotoCurator = (*Client)(nil)

// New prepara a conexão com o curador em addr (ex.: "photo-curator:50061").
// grpc.NewClient não conecta na hora: a conexão abre na primeira chamada e
// se refaz sozinha se o curador reiniciar.
//
// Sem TLS ("insecure"): o curador só é alcançável pela rede passarim-curation
// do compose, que é interna e só tem ele e o worker.
func New(addr string, timeout time.Duration) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("curador: configurar conexão: %w", err)
	}
	return newClient(conn, timeout), nil
}

// newClient monta o cliente sobre uma conexão pronta (os testes passam uma
// conexão em memória).
func newClient(conn *grpc.ClientConn, timeout time.Duration) *Client {
	return &Client{conn: conn, api: curatorv1.NewPhotoCuratorServiceClient(conn), timeout: timeout}
}

// Close fecha a conexão.
func (c *Client) Close() error { return c.conn.Close() }

// ClassifyPhotos manda um lote de até 8 fotos e devolve os vereditos na
// ordem do pedido, mais a versão do modelo.
func (c *Client) ClassifyPhotos(ctx context.Context, photos []ingest.CuratorPhoto) ([]ingest.PhotoVerdict, string, error) {
	if len(photos) == 0 || len(photos) > ingest.MaxCuratorBatch {
		return nil, "", fmt.Errorf("curador: lote de %d fotos (o contrato aceita de 1 a %d)", len(photos), ingest.MaxCuratorBatch)
	}
	req := &curatorv1.ClassifyPhotosRequest{}
	for _, p := range photos {
		req.Photos = append(req.Photos, &curatorv1.PhotoInput{PhotoId: p.PhotoID, Image: p.Image})
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := c.api.ClassifyPhotos(ctx, req)
	if err != nil {
		return nil, "", translate(err)
	}
	// O contrato promete um veredito por foto, na mesma ordem. Se a resposta
	// não bater, é melhor falhar do que dar a categoria de uma foto a outra.
	if len(resp.GetVerdicts()) != len(photos) {
		return nil, "", fmt.Errorf("curador: %d vereditos para %d fotos", len(resp.GetVerdicts()), len(photos))
	}
	out := make([]ingest.PhotoVerdict, len(photos))
	for i, v := range resp.GetVerdicts() {
		if v.GetPhotoId() != photos[i].PhotoID {
			return nil, "", fmt.Errorf("curador: veredito %d é da foto %q, esperava %q", i, v.GetPhotoId(), photos[i].PhotoID)
		}
		out[i] = ingest.PhotoVerdict{PhotoID: v.GetPhotoId(), Category: category(v.GetCategory()),
			BirdCount: int(v.GetBirdCount()), LargestBirdArea: float64(v.GetLargestBirdArea()),
			LargestBirdCut: v.GetLargestBirdCut(), Sharpness: float64(v.GetSharpness()), Confidence: float64(v.GetConfidence())}
	}
	return out, resp.GetModelVersion(), nil
}

// translate separa "curador fora do ar" (adiar o job) de "erro de verdade"
// (gastar tentativa). status.Code lê o código gRPC do erro.
//   - UNAVAILABLE: o curador não está no ar ou está subindo.
//   - DEADLINE_EXCEEDED: não respondeu no prazo.
//   - CANCELED: quem chamou desistiu (SIGTERM ou fim do prazo do job).
func translate(err error) error {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
		return errors.Join(ingest.ErrCuratorUnavailable, err)
	}
	return fmt.Errorf("curador: %w", err)
}

// categories traduz o enum do contrato para o domínio.
var categories = map[curatorv1.PhotoCategory]domain.PhotoCategory{
	curatorv1.PhotoCategory_PHOTO_CATEGORY_SINGLE_BIRD: domain.PhotoSingleBird,
	curatorv1.PhotoCategory_PHOTO_CATEGORY_FLOCK:       domain.PhotoFlock,
	curatorv1.PhotoCategory_PHOTO_CATEGORY_NEST:        domain.PhotoNest,
	curatorv1.PhotoCategory_PHOTO_CATEGORY_EGGS:        domain.PhotoEggs,
	curatorv1.PhotoCategory_PHOTO_CATEGORY_FEATHER:     domain.PhotoFeather,
	curatorv1.PhotoCategory_PHOTO_CATEGORY_DEAD_BIRD:   domain.PhotoDeadBird,
	curatorv1.PhotoCategory_PHOTO_CATEGORY_NO_BIRD:     domain.PhotoNoBird,
	curatorv1.PhotoCategory_PHOTO_CATEGORY_OTHER:       domain.PhotoOther,
}

// category traduz uma categoria. Um número que não conhecemos (de uma
// versão futura do contrato) vira unspecified, que é tratado como descarte.
func category(c curatorv1.PhotoCategory) domain.PhotoCategory {
	if d, ok := categories[c]; ok {
		return d
	}
	return domain.PhotoUnspecified
}
