# passarim-catalog

Catalog API do **Passarim**: guarda as aves do Brasil no PostgreSQL e as serve via **gRPC** para o BFF.
Todo o código é comentado para estudo. Arquitetura geral: [passarim-docs](https://github.com/velosobr/passarim-docs).

## Arquitetura limpa

```mermaid
flowchart LR
    subgraph adapter["adapter (entrada)"]
        G[grpc<br/>Server]
        Y[curated<br/>YAML loader]
    end
    subgraph usecase
        L[ListSpecies]
        D[GetSpecies]
        F[ListFilters]
        P{{SpeciesRepository<br/>interface}}
    end
    subgraph domain
        S[Species · Biome · UF<br/>regras puras]
    end
    subgraph adapter2["adapter (saída)"]
        R[postgres<br/>Repository]
    end
    G --> L & D & F
    L & D & F --> P
    L & D & F --> S
    R -. implementa .-> P
    R --> DB[(PostgreSQL)]
    Y --> S
```

As setas só apontam **para dentro**: `domain` não conhece banco nem gRPC.

## Rodando
| Comando | O que faz |
|---|---|
| `make test` | Testes (os de integração sobem um PostgreSQL via Docker) |
| `make generate` | Gera o código das queries (sqlc) |
| `make lint` | golangci-lint |
| `make run` | Sobe a API apontando para o PostgreSQL do compose |

Tudo junto (banco + API + seed): veja o `docker compose` do [passarim-docs](https://github.com/velosobr/passarim-docs).

## Conteúdo
As 40 aves curadas ficam em [`content/species`](content/species) — veja as [regras de escrita](content/README.md).
