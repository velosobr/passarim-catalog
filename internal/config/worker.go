package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// WorkerConfig é a configuração do binário cmd/worker.
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
}

// LoadWorker lê o ambiente. Os erros citam o NOME da variável, nunca o
// valor: segredos não podem vazar para logs.
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
	return c, nil
}

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
