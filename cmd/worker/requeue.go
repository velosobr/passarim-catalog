// Leitura dos argumentos do comando "worker requeue". Fica num arquivo
// próprio para ser testada sem banco.

package main

import (
	"errors"
	"flag"
	"io"

	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// parseRequeueArgs lê "[--source photos|audio] [species-id...]". O padrão é
// photos, para o comando que o README já documenta continuar igual. O
// pacote flag para de ler opções no primeiro argumento que não começa com
// "-": por isso o --source vem antes dos ids.
func parseRequeueArgs(args []string) (ingest.Source, []string, error) {
	fs := flag.NewFlagSet("requeue", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o erro volta para quem chamou, sem imprimir a ajuda
	name := fs.String("source", "photos", "photos ou audio")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	sources := map[string]ingest.Source{"photos": ingest.SourceINaturalist, "audio": ingest.SourceXenoCanto}
	source, ok := sources[*name]
	if !ok {
		return "", nil, errors.New("--source deve ser photos ou audio")
	}
	return source, fs.Args(), nil
}
