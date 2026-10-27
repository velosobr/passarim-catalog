// Package ingest contém os casos de uso do worker de ingestão: buscar
// mídia e avistamentos em fontes externas e gravar no nosso catálogo.
// Como no resto do projeto, aqui só há regras e interfaces ("ports");
// HTTP, ffmpeg, S3 e SQL ficam nos adapters.
//
// Este arquivo define as portas da ingestão: a fila de jobs, o repositório
// de mídia, as três fontes, o downloader, os conversores e o storage. Cada
// interface descreve só o que o caso de uso precisa, e os testes usam
// versões falsas de todas elas (fakes_test.go).
package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// Source identifica de onde vem cada tipo de dado.
type Source string

const (
	SourceINaturalist Source = "inaturalist" // fotos
	SourceXenoCanto   Source = "xenocanto"   // cantos
	SourceGBIF        Source = "gbif"        // avistamentos
)

// AllSources na ordem em que os jobs são criados.
var AllSources = []Source{SourceINaturalist, SourceXenoCanto, SourceGBIF}

// Job é uma tarefa "buscar <fonte> para <espécie>".
type Job struct {
	ID             int64
	SpeciesID      string
	ScientificName string
	Source         Source
	Attempts       int
}

// JobQueue é a fila persistente (tabela ingestion_job).
type JobQueue interface {
	// EnqueueMissing faz a manutenção da fila: cria os jobs que ainda não
	// existem (espécie × fonte), reagenda os concluídos há mais de
	// refreshAfter, reabre os failed antigos e devolve à fila os presos em
	// running. Devolve quantos ficaram pendentes.
	EnqueueMissing(ctx context.Context, sources []Source, refreshAfter time.Duration) (int, error)
	// ClaimDue pega até limit jobs vencidos e os marca como "running",
	// sem que outro worker pegue os mesmos.
	ClaimDue(ctx context.Context, limit int) ([]Job, error)
	// Complete marca o job como concluído com sucesso.
	Complete(ctx context.Context, jobID int64) error
	// Release devolve à fila um job que foi pego mas não chegou a rodar
	// (ex.: o lote estourou o tempo). Não gasta tentativa.
	Release(ctx context.Context, jobID int64) error
	// Fail registra o erro e conta uma tentativa. Com giveUp, status = failed;
	// senão volta para pending com next_run_at = agora + retryIn.
	Fail(ctx context.Context, jobID int64, cause string, retryIn time.Duration, giveUp bool) error
}

// MediaRepository grava o resultado da ingestão. "Replace" = apaga o que
// havia DAQUELE tipo para a espécie e grava o novo, numa transação.
// Assim reprocessar nunca duplica nada.
type MediaRepository interface {
	// ReplacePhotos troca as fotos da espécie e grava a linha do relatório
	// de curadoria, as duas coisas numa transação só.
	ReplacePhotos(ctx context.Context, speciesID string, photos []domain.Photo, report domain.CurationReport) error
	ReplaceAudio(ctx context.Context, speciesID string, audio *domain.Audio) error
	ReplaceClusters(ctx context.Context, speciesID string, clusters []domain.OccurrenceCluster) error
}

// PhotoCandidate é uma foto encontrada na fonte (ainda não baixada).
type PhotoCandidate struct {
	SourceID      string // id da foto na fonte; faz parte da chave no storage
	ObservationID string // id da observação; a galeria aceita até 2 fotos de cada
	Votes         int    // votos da observação (faves); fotos da mesma observação empatam
	URL           string // URL do arquivo em tamanho grande
	PageURL       string // página da observação (crédito)
	License       string // como a fonte informa (ex.: "cc-by-nc")
	Author        string
	Width         int
	Height        int
}

// AudioCandidate é uma gravação encontrada na fonte.
type AudioCandidate struct {
	SourceID   string // id da gravação na fonte; faz parte da chave no storage
	URL        string // download do arquivo
	PageURL    string
	License    string // URL da licença, como o xeno-canto informa
	Author     string
	Quality    string // "A" (melhor) a "E"
	Type       string // "song", "call"...
	DurationMs int
}

// As três fontes externas: fotos (iNaturalist), cantos (xeno-canto) e
// avistamentos (GBIF). Cada uma busca pelo nome científico.
type PhotoSource interface {
	// FindPhotos busca até `observations` observações da espécie e devolve
	// até 3 fotos de cada, sem as observações anotadas como morto, ovo, pena,
	// osso, muda ou ninho.
	FindPhotos(ctx context.Context, scientificName string, observations int) ([]PhotoCandidate, error)
	// FindObservationPhoto busca uma foto dentro de uma observação (a
	// principal escolhida no YAML). ok=false: a observação ou a foto não existe.
	FindObservationPhoto(ctx context.Context, observationID, photoID string) (PhotoCandidate, bool, error)
}

type AudioSource interface {
	FindRecordings(ctx context.Context, scientificName string) ([]AudioCandidate, error)
}

type OccurrenceSource interface {
	FindOccurrences(ctx context.Context, scientificName string, max int) ([]domain.Point, error)
}

// Downloader baixa um arquivo de forma segura (anti-SSRF) e devolve os bytes e o Content-Type.
type Downloader interface {
	Fetch(ctx context.Context, url string, kind DownloadKind) ([]byte, string, error)
}

// DownloadKind define limites diferentes para imagem e áudio.
type DownloadKind int

const (
	// iota numera as constantes a partir de 0: DownloadImage = 0, DownloadAudio = 1.
	DownloadImage DownloadKind = iota
	DownloadAudio
)

// PhotoVariants são as três versões WebP de uma foto.
type PhotoVariants struct {
	Thumb, Medium, Large []byte
	Width, Height        int // dimensões da versão Large
}

// ImageProcessor gera as versões WebP de uma foto original, em dois passos:
// a medium de todas as candidatas (é ela que vai ao curador) e a thumb e a
// large só das escolhidas. Assim as ~40 candidatas não custam 3 conversões
// cada.
type ImageProcessor interface {
	// Medium gera só a variante de 800 px.
	Medium(ctx context.Context, original []byte) ([]byte, error)
	// ThumbAndLarge gera a thumb (320 px) e a large (1600 px), com as
	// dimensões da large. O campo Medium volta vazio.
	ThumbAndLarge(ctx context.Context, original []byte) (PhotoVariants, error)
}

// AudioProcessor converte gravações para o formato do app.
type AudioProcessor interface {
	// ToAAC converte para AAC mono e corta em maxDuration.
	ToAAC(ctx context.Context, original []byte, maxDuration time.Duration) ([]byte, error)
}

// MediaStore é o object storage (SeaweedFS local / R2 em produção).
type MediaStore interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	// List devolve as chaves que começam com prefix.
	List(ctx context.Context, prefix string) ([]string, error)
	// Delete apaga as chaves dadas (chave inexistente não é erro).
	Delete(ctx context.Context, keys []string) error
}

// ErrCuratorUnavailable: o curador está fora do ar, subindo ou não
// respondeu no prazo. O job de fotos devolve este erro ANTES de trocar
// qualquer foto, e o Runner adia o job sem gastar tentativa (é uma falha
// do nosso lado, não da espécie).
var ErrCuratorUnavailable = errors.New("curador indisponível")

// MaxCuratorBatch é o maior lote que o curador aceita (o contrato diz 8).
const MaxCuratorBatch = 8

// CuratorPhoto é uma foto mandada ao curador: o id (ecoado na resposta) e
// os bytes WebP da variante medium.
type CuratorPhoto struct {
	PhotoID string
	Image   []byte
}

// PhotoVerdict é o que o curador diz de uma foto (espelha o PhotoVerdict do
// curator.proto, sem tipos do gRPC).
type PhotoVerdict struct {
	PhotoID         string
	Category        domain.PhotoCategory
	BirdCount       int
	LargestBirdArea float64 // fração da foto ocupada pela maior ave, de 0 a 1
	LargestBirdCut  bool    // a maior ave encosta na borda
	Sharpness       float64 // de 0 a 1
	Confidence      float64 // de 0 a 1
}

// PhotoCurator classifica até MaxCuratorBatch fotos por chamada. Devolve os
// vereditos na ordem do pedido e a versão do modelo. Curador fora do ar ou
// sem resposta no prazo: ErrCuratorUnavailable.
type PhotoCurator interface {
	ClassifyPhotos(ctx context.Context, photos []CuratorPhoto) ([]PhotoVerdict, string, error)
}

// OverrideSource lê a escolha manual de fotos (chave photos: do YAML, que o
// seed grava). Sem escolha, devolve o override vazio.
type OverrideSource interface {
	PhotoOverride(ctx context.Context, speciesID string) (domain.PhotoOverride, error)
}
