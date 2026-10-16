// Package config lê a configuração das variáveis de ambiente.
// Padrão "12-factor app": a mesma imagem Docker roda em qualquer ambiente;
// só as variáveis mudam.
//
// Este arquivo é a configuração da API (cmd/catalog-api); worker.go tem a do
// worker. As duas compartilham os helpers or e boolVar.
package config

import (
	"errors"
	"fmt"
	"strconv"
)

// Config é a configuração da API, já lida e validada. Só DATABASE_URL é
// obrigatória; o resto tem padrão (gRPC em :50051, métricas em :9091,
// migrations ligadas, reflection desligada).
type Config struct {
	DatabaseURL    string // ex.: postgres://user:senha@host:5432/db?sslmode=disable
	GRPCAddr       string // onde o gRPC escuta
	MetricsAddr    string // onde /metrics e /healthz escutam (HTTP)
	MigrateOnStart bool   // aplica migrations ao subir
	Reflection     bool   // gRPC reflection (só em desenvolvimento: deixa o grpcurl descobrir os métodos)
}

// Load recebe getenv como parâmetro (e não chama os.Getenv direto) para
// que os testes passem um "ambiente" falso.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		DatabaseURL: getenv("DATABASE_URL"),
		GRPCAddr:    or(getenv("GRPC_ADDR"), ":50051"),
		MetricsAddr: or(getenv("METRICS_ADDR"), ":9091"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL é obrigatória")
	}
	var err error
	if c.MigrateOnStart, err = boolVar(getenv, "MIGRATE_ON_START", true); err != nil {
		return c, err
	}
	if c.Reflection, err = boolVar(getenv, "GRPC_REFLECTION", false); err != nil {
		return c, err
	}
	return c, nil
}

// or devolve v, ou def quando v está vazio.
func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// boolVar lê um booleano. strconv.ParseBool aceita "true", "false", "1",
// "0", "t", "f" e variações em maiúsculas.
func boolVar(getenv func(string) string, key string, def bool) (bool, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: esperava true/false, veio %q", key, v)
	}
	return b, nil
}
