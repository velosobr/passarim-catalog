# Atalhos do dia a dia. Uso: make test | make generate | make lint | make run
.PHONY: test generate lint run

# Todos os testes. Os de integração sobem um PostgreSQL de verdade (precisa de Docker).
test:
	go test -race ./...

# Gera o código Go das queries SQL (sqlc).
generate:
	go tool sqlc generate

lint:
	golangci-lint run

# Roda a API localmente apontando para o PostgreSQL do docker compose.
run:
	DATABASE_URL=postgres://passarim:passarim-dev@localhost:5432/passarim?sslmode=disable go run ./cmd/catalog-api
