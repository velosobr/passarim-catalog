// Configuração do worker (cmd/worker).

package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// WorkerConfig é a configuração do binário cmd/worker.
//
// Obrigatórias: DATABASE_URL, S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY e
// XENO_CANTO_API_KEY. Padrões: bucket "passarim-media", sem SSL no S3,
// aceita licenças NC, rodadas a cada 1 min com 5 jobs, métricas em :9092,
// "ffmpeg" no PATH e um User-Agent que identifica o projeto. Mandar um
// User-Agent com contato é uma regra de boa educação das APIs públicas.
// Curadoria: o curador em photo-curator:50061, prazo de 6 min por job de
// fotos e LOW_CONFIDENCE calibrado no conjunto de referência do curador.
type WorkerConfig struct {
	DatabaseURL     string
	S3Endpoint      string
	S3AccessKey     string
	S3SecretKey     string
	S3Bucket        string
	S3UseSSL        bool
	XenoCantoAPIKey string
	Interval        time.Duration // pausa entre rodadas quando não há trabalho
	BatchSize       int           // jobs por rodada
	AllowNC         bool          // aceitar licenças NonCommercial
	MetricsAddr     string
	FFmpegPath      string
	UserAgent       string
	CuratorAddr     string        // endereço gRPC do passarim-photo-curator
	PhotoJobTimeout time.Duration // prazo de um job de fotos
	LowConfidence   float64       // abaixo disso, a principal é marcada no relatório
}

// DefaultLowConfidence é o percentil 20 da confiança das fotos que o curador
// chama de ave sozinha na metade de calibração do conjunto de referência
// (scripts/low_confidence.py do passarim-photo-curator), medido com
// rtdetr-r50vd+openclip-b32@2. Recalcule quando o MODEL_VERSION do curador mudar.
const DefaultLowConfidence = 0.872

// LoadWorker lê o ambiente. Os erros das obrigatórias citam só o NOME da
// variável, nunca o valor: segredos não podem vazar para logs. Só os
// booleanos, que não são segredo, mostram o valor recebido.
func LoadWorker(getenv func(string) string) (WorkerConfig, error) {
	c := WorkerConfig{
		DatabaseURL:     getenv("DATABASE_URL"),
		S3Endpoint:      getenv("S3_ENDPOINT"),
		S3AccessKey:     getenv("S3_ACCESS_KEY"),
		S3SecretKey:     getenv("S3_SECRET_KEY"),
		S3Bucket:        or(getenv("S3_BUCKET"), "passarim-media"),
		XenoCantoAPIKey: getenv("XENO_CANTO_API_KEY"),
		MetricsAddr:     or(getenv("METRICS_ADDR"), ":9092"),
		FFmpegPath:      or(getenv("FFMPEG_PATH"), "ffmpeg"),
		UserAgent:       or(getenv("USER_AGENT"), "Passarim/0.1 (+https://github.com/velosobr/passarim-docs)"),
		CuratorAddr:     or(getenv("CURATOR_ADDR"), "photo-curator:50061"),
	}
	for name, v := range map[string]string{
		"DATABASE_URL": c.DatabaseURL, "S3_ENDPOINT": c.S3Endpoint, "S3_ACCESS_KEY": c.S3AccessKey,
		"S3_SECRET_KEY": c.S3SecretKey, "XENO_CANTO_API_KEY": c.XenoCantoAPIKey,
	} {
		if v == "" {
			return c, errors.New(name + " é obrigatória")
		}
	}
	var err error
	if c.S3UseSSL, err = boolVar(getenv, "S3_USE_SSL", false); err != nil {
		return c, err
	}
	if c.AllowNC, err = boolVar(getenv, "ALLOW_NC", true); err != nil {
		return c, err
	}
	if c.Interval, err = durationVar(getenv, "WORKER_INTERVAL", time.Minute); err != nil {
		return c, err
	}
	if v := getenv("WORKER_BATCH_SIZE"); v == "" {
		c.BatchSize = 5
	} else if c.BatchSize, err = strconv.Atoi(v); err != nil || c.BatchSize < 1 {
		return c, errors.New("WORKER_BATCH_SIZE: esperava um inteiro positivo")
	}
	if c.PhotoJobTimeout, err = durationVar(getenv, "PHOTO_JOB_TIMEOUT", 6*time.Minute); err != nil {
		return c, err
	}
	c.LowConfidence = DefaultLowConfidence
	if v := getenv("LOW_CONFIDENCE"); v != "" {
		// Uma probabilidade: estritamente entre 0 e 1 (0 não marcaria nada; 1 marcaria tudo).
		if c.LowConfidence, err = strconv.ParseFloat(v, 64); err != nil || c.LowConfidence <= 0 || c.LowConfidence >= 1 {
			return c, errors.New("LOW_CONFIDENCE: esperava um número entre 0 e 1, ex.: 0.8")
		}
	}
	return c, nil
}

// durationVar lê uma duração no formato do Go ("30s", "1m"); zero ou
// negativa é erro.
func durationVar(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: esperava uma duração positiva, ex.: 1m", key)
	}
	return d, nil
}
