# Conteúdo curado

Cada arquivo em `species/` descreve uma ave. Nome do arquivo = id (nome científico em minúsculas com hífen).

## Regras
- **Escreva com suas palavras.** Não copie textos da Wikipedia ou de outros sites — use-os como referência e cite o link.
- Descrição: 200 a 600 caracteres, em português, tom de guia de campo para leigos.
- Curiosidades: de 2 a 4, cada uma com o link (`https://`) de onde a informação foi conferida.
- `conservation_status`: categoria da IUCN conferida na Wikipedia (caixa "Estado de conservação") ou em iucnredlist.org.
- `states` e `biomes`: onde a ave ocorre de forma regular no Brasil (conferir no WikiAves ou Wikipedia).
- Na dúvida sobre um dado, **deixe de fora** em vez de chutar (`size_cm` e `diet` são opcionais).

## Formato
Veja `species/turdus-rufiventris.yaml`.

## Conteúdo Kids (`primary_biome:` e `kids:`)

A linha `primary_biome:` e o bloco `kids:` são escritos pelo `cmd/kidsgen` (frente B) e revisados por uma pessoa no PR. O `go test ./content/` roda o validador em toda espécie que tem o bloco: frases (descrição de 2 a 4, fatos e "Você sabia?" com 1, até 15 palavras cada), números (todo número do Kids precisa estar no texto adulto), palavras proibidas e o `source_hash`. Sem o bloco, nada é exigido.

- Gerar para as espécies que faltam: `ANTHROPIC_API_KEY=... go run ./cmd/kidsgen --missing`. A chave fica só na sua máquina.
- Uma espécie: `--ids turdus-rufiventris`. Ver o prompt sem gastar: `--dry-run`, sem a chave.
- Mudou a descrição, um fato ou os biomas? O `source_hash` deixa de bater, o teste falha e pede uma nova geração. Mudar só as fontes, o tamanho ou as UFs não invalida.
- O bioma principal pode ser trocado à mão (ele não entra no `source_hash`), desde que continue entre os `biomes`.
