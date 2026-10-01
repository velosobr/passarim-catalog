# Build em dois estágios ("multi-stage"):
# 1) uma imagem com o Go completo compila o binário;
# 2) a imagem final leva SÓ o binário — menor e com menos superfície de ataque.

FROM golang:1.27-alpine AS build
WORKDIR /src
# Copiar go.mod/go.sum primeiro aproveita o cache do Docker: as dependências
# só são baixadas de novo quando esses arquivos mudam.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=0 gera binário estático (não depende de bibliotecas do sistema).
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/catalog-api ./cmd/catalog-api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# distroless: sem shell, sem gerenciador de pacotes. "nonroot" = não roda como root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /usr/local/bin/
COPY content /content
USER nonroot:nonroot
EXPOSE 50051 9091
ENTRYPOINT ["/usr/local/bin/catalog-api"]
