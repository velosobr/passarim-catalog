// Package ingest contém os casos de uso do worker de ingestão: buscar
// mídia e avistamentos em fontes externas e gravar no nosso catálogo.
// Como no resto do projeto, aqui só há regras e interfaces ("ports");
// HTTP, ffmpeg, S3 e SQL ficam nos adapters.
package ingest

import (
	"context"
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
	// EnqueueMissing cria jobs que ainda não existem (espécie × fonte) e
	// reagenda jobs concluídos há mais de refreshAfter. Devolve quantos ficaram pendentes.
	EnqueueMissing(ctx context.Context, sources []Source, refreshAfter time.Duration) (int, error)
	// ClaimDue pega até limit jobs vencidos e os marca como "running",
	// sem que outro worker pegue os mesmos.
	ClaimDue(ctx context.Context, limit int) ([]Job, error)
	Complete(ctx context.Context, jobID int64) error
	// Release devolve à fila um job que foi pego mas não chegou a rodar
	// (ex.: o lote estourou o tempo). Não gasta tentativa.
	Release(ctx context.Context, jobID int64) error
	// Fail registra o erro. Se attempts chegou ao máximo, status = failed;
	// senão volta para pending com next_run_at = agora + retryIn.
	Fail(ctx context.Context, jobID int64, cause string, retryIn time.Duration, giveUp bool) error
}

// MediaRepository grava o resultado da ingestão. "Replace" = apaga o que
// havia DAQUELE tipo para a espécie e grava o novo, numa transação.
// Assim reprocessar nunca duplica nada.
type MediaRepository interface {
	ReplacePhotos(ctx context.Context, speciesID string, photos []domain.Photo) error
	ReplaceAudio(ctx context.Context, speciesID string, audio *domain.Audio) error
	ReplaceClusters(ctx context.Context, speciesID string, clusters []domain.OccurrenceCluster) error
}

// PhotoCandidate é uma foto encontrada na fonte (ainda não baixada).
type PhotoCandidate struct {
	SourceID string // id da foto na fonte; faz parte da chave no storage
	URL      string // URL do arquivo em tamanho grande
	PageURL  string // página da observação (crédito)
	License  string // como a fonte informa (ex.: "cc-by-nc")
	Author   string
	Width    int
	Height   int
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

type PhotoSource interface {
	FindPhotos(ctx context.Context, scientificName string, max int) ([]PhotoCandidate, error)
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
	DownloadImage DownloadKind = iota
	DownloadAudio
)

// PhotoVariants são as três versões WebP de uma foto.
type PhotoVariants struct {
	Thumb, Medium, Large []byte
	Width, Height        int // dimensões da versão Large
}

type ImageProcessor interface {
	Process(ctx context.Context, original []byte) (PhotoVariants, error)
}

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
