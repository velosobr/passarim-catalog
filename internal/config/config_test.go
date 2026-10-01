package config_test

import (
	"testing"

	"github.com/velosobr/passarim-catalog/internal/config"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoad_Defaults(t *testing.T) {
	c, err := config.Load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.GRPCAddr != ":50051" || c.MetricsAddr != ":9091" || !c.MigrateOnStart || c.Reflection {
		t.Fatalf("padrões errados: %+v", c)
	}
}

func TestLoad_Overrides(t *testing.T) {
	c, _ := config.Load(env(map[string]string{"DATABASE_URL": "postgres://x", "GRPC_ADDR": ":1", "METRICS_ADDR": ":2",
		"MIGRATE_ON_START": "false", "GRPC_REFLECTION": "true"}))
	if c.GRPCAddr != ":1" || c.MetricsAddr != ":2" || c.MigrateOnStart || !c.Reflection {
		t.Fatalf("overrides ignorados: %+v", c)
	}
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	if _, err := config.Load(env(nil)); err == nil {
		t.Fatal("DATABASE_URL é obrigatória")
	}
}

func TestLoad_RejectsBadBool(t *testing.T) {
	if _, err := config.Load(env(map[string]string{"DATABASE_URL": "x", "GRPC_REFLECTION": "talvez"})); err == nil {
		t.Fatal("booleano inválido deveria dar erro")
	}
}
