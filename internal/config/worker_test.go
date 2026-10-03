package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/config"
)

func required() map[string]string {
	return map[string]string{
		"DATABASE_URL": "postgres://h/db", "S3_ENDPOINT": "s3:8333", "S3_ACCESS_KEY": "ak",
		"S3_SECRET_KEY": "sk", "XENO_CANTO_API_KEY": "xc",
	}
}

func TestLoadWorker_Defaults(t *testing.T) {
	c, err := config.LoadWorker(env(required()))
	if err != nil {
		t.Fatal(err)
	}
	if c.S3Bucket != "passarim-media" || c.Interval != time.Minute || c.BatchSize != 5 || !c.AllowNC ||
		c.MetricsAddr != ":9092" || c.FFmpegPath != "ffmpeg" || !strings.HasPrefix(c.UserAgent, "Passarim/0.1") || c.S3UseSSL {
		t.Fatalf("padrões errados: %+v", c)
	}
}

func TestLoadWorker_MissingRequiredNamesTheVariable(t *testing.T) {
	for _, name := range []string{"DATABASE_URL", "S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY", "XENO_CANTO_API_KEY"} {
		m := required()
		delete(m, name)
		if _, err := config.LoadWorker(env(m)); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("sem %s: erro deveria citar o nome, veio %v", name, err)
		}
	}
}

func TestLoadWorker_InvalidValues(t *testing.T) {
	for k, v := range map[string]string{"WORKER_INTERVAL": "abc", "WORKER_BATCH_SIZE": "x", "ALLOW_NC": "talvez", "S3_USE_SSL": "?"} {
		m := required()
		m[k] = v
		if _, err := config.LoadWorker(env(m)); err == nil || !strings.Contains(err.Error(), k) {
			t.Errorf("%s=%s deveria dar erro citando a variável: %v", k, v, err)
		}
	}
}

func TestLoadWorker_ErrorNeverContainsSecretValues(t *testing.T) {
	m := required()
	m["S3_SECRET_KEY"] = "super-secreto"
	m["XENO_CANTO_API_KEY"] = "chave-xeno"
	m["WORKER_INTERVAL"] = "xx"
	_, err := config.LoadWorker(env(m))
	if err == nil || strings.Contains(err.Error(), "super-secreto") || strings.Contains(err.Error(), "chave-xeno") {
		t.Fatalf("erro não pode conter segredos: %v", err)
	}
}
