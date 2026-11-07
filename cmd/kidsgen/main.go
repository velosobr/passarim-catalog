// cmd/kidsgen gera o conteúdo Kids (frente B): para cada espécie escolhida,
// pede ao modelo um texto para crianças a partir do texto adulto curado,
// confere com o validador do conteúdo e grava o bloco kids: e a linha
// primary_biome: no YAML. Roda à mão, na máquina de quem cuida do conteúdo,
// com a ANTHROPIC_API_KEY dela; a CI nunca chama a API. O resultado é
// revisado por uma pessoa no PR (ADR-0022).
//
// Uso:
//
//	ANTHROPIC_API_KEY=... go run ./cmd/kidsgen --missing
//	ANTHROPIC_API_KEY=... go run ./cmd/kidsgen --ids turdus-rufiventris,crotophaga-ani
//	go run ./cmd/kidsgen --ids turdus-rufiventris --dry-run
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/velosobr/passarim-catalog/internal/adapter/curated"
	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/kidscontent"
)

// starter abre uma conversa com o modelo para uma espécie. Em produção é
// newAnthropicConversation; nos testes, um falso.
type starter func(apiKey string, s domain.Species) conversation

func main() {
	// NotifyContext cancela o contexto no Ctrl+C: a chamada em andamento é
	// interrompida e nada pela metade é gravado.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv("ANTHROPIC_API_KEY"), os.Stdout, newAnthropicConversation); err != nil {
		fmt.Fprintln(os.Stderr, "kidsgen:", err)
		os.Exit(1)
	}
}

// run é o programa inteiro, com as dependências recebidas por parâmetro
// para os testes: os argumentos, a chave, onde escrever e como abrir a
// conversa com o modelo.
func run(ctx context.Context, args []string, apiKey string, out io.Writer, start starter) error {
	fs := flag.NewFlagSet("kidsgen", flag.ContinueOnError)
	fs.SetOutput(out)
	dir := fs.String("dir", "content/species", "pasta dos YAMLs das espécies")
	missing := fs.Bool("missing", false, "gera para as espécies sem bloco kids: ou com o texto adulto mudado")
	ids := fs.String("ids", "", "ids das espécies, separados por vírgula")
	dryRun := fs.Bool("dry-run", false, "não grava; sem a chave, só mostra o prompt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *missing == (*ids != "") {
		return errors.New("use --missing ou --ids (um dos dois)")
	}
	if apiKey == "" && !*dryRun {
		return errors.New("ANTHROPIC_API_KEY não definida (só o --dry-run roda sem ela)")
	}

	species, err := curated.LoadDir(*dir)
	if err != nil {
		return err
	}
	selected, err := selectSpecies(species, *missing, *ids)
	if err != nil {
		return err
	}
	if apiKey == "" {
		_, _ = fmt.Fprintf(out, "== prompt do sistema ==\n%s\n", systemPrompt)
	}

	var failed []string
	for _, s := range selected {
		// Sem texto adulto, nenhuma saída passaria no validador: pular antes
		// de chamar a API evita pagar 3 chamadas sem chance de dar certo.
		if strings.TrimSpace(s.Description) == "" || len(s.Facts) == 0 {
			_, _ = fmt.Fprintf(out, "%s: pulada, sem texto adulto curado\n", s.ID)
			continue
		}
		if apiKey == "" {
			_, _ = fmt.Fprintf(out, "== %s ==\n%s\n", s.ID, userMessage(s))
			continue
		}
		res, err := generate(ctx, start(apiKey, s), s)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s: FALHOU: %v\n", s.ID, err)
			failed = append(failed, s.ID)
			continue
		}
		// Esta linha vai para a descrição do PR de conteúdo, para a revisão do bioma.
		_, _ = fmt.Fprintf(out, "%s → primary_biome: %s (%s)\n", s.ID, res.species.PrimaryBiome, res.reason)
		if *dryRun {
			_, _ = fmt.Fprint(out, kidsBlock(*res.species.Kids))
			continue
		}
		if err := writeKids(filepath.Join(*dir, s.ID+".yaml"), res.species); err != nil {
			return err
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d espécie(s) puladas: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

// selectSpecies escolhe as espécies da rodada. Com --missing, as que não têm
// bloco kids: ou cujo carimbo do texto adulto mudou. Com --ids, as pedidas;
// um id que não existe é erro, para um erro de digitação não passar calado.
func selectSpecies(all []domain.Species, missing bool, ids string) ([]domain.Species, error) {
	if missing {
		var out []domain.Species
		for _, s := range all {
			if s.Kids == nil || s.Kids.SourceHash != kidscontent.SourceHash(s) {
				out = append(out, s)
			}
		}
		return out, nil
	}
	byID := map[string]domain.Species{}
	for _, s := range all {
		byID[s.ID] = s
	}
	var out []domain.Species
	for _, id := range strings.Split(ids, ",") {
		s, ok := byID[strings.TrimSpace(id)]
		if !ok {
			return nil, fmt.Errorf("espécie %q não existe", strings.TrimSpace(id))
		}
		out = append(out, s)
	}
	return out, nil
}

// writeKids grava o resultado no YAML da espécie, por edição de texto.
func writeKids(path string, s domain.Species) error {
	src, err := os.ReadFile(path) //nolint:gosec // G304: o caminho é a pasta do conteúdo + o id de uma espécie já carregada
	if err != nil {
		return err
	}
	edited, err := applyKids(src, s.PrimaryBiome, *s.Kids)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	// 0o600 só vale para arquivo novo; um YAML que já existe mantém as
	// permissões que tinha.
	return os.WriteFile(path, edited, 0o600) //nolint:gosec // G703: o mesmo caminho lido acima
}
