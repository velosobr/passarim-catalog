// Gravação do resultado do kidsgen no YAML da espécie, por edição de TEXTO
// (spec da frente B, seção 3.5). Regravar o arquivo inteiro com um
// codificador YAML trocaria o estilo ">" da descrição, a ordem das chaves e
// apagaria os comentários; editando só as linhas certas, o diff do PR de
// conteúdo fica pequeno e fácil de revisar.

package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// (?m) faz ^ e $ valerem para cada linha, não só para o texto inteiro.
var (
	biomesLine   = regexp.MustCompile(`(?m)^biomes:.*$`)
	biomesInline = regexp.MustCompile(`^biomes:\s*\[[^\]]*\]\s*(#.*)?$`)
	primaryLine  = regexp.MustCompile(`(?m)^primary_biome:.*\n`)
	kidsLine     = regexp.MustCompile(`(?m)^kids:.*\n`)
)

// applyKids devolve o arquivo com a linha primary_biome: e o bloco kids:
// novos:
//   - primary_biome: é trocada se existir; senão, entra logo depois de biomes:;
//   - o bloco kids: é trocado se existir; senão, entra no fim do arquivo.
//
// biomes: precisa estar numa linha só ("[a, b]"), que é o formato de todos
// os YAMLs do catálogo; em outro formato, applyKids recusa em vez de chutar
// onde a lista termina.
func applyKids(src []byte, primary domain.Biome, k domain.KidsContent) ([]byte, error) {
	text := string(src)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	loc := biomesLine.FindStringIndex(text)
	if loc == nil {
		return nil, errors.New("chave biomes: não encontrada")
	}
	if !biomesInline.MatchString(text[loc[0]:loc[1]]) {
		return nil, errors.New("biomes: precisa estar numa linha só, no formato [a, b]")
	}
	line := "primary_biome: " + string(primary) + "\n"
	if m := primaryLine.FindStringIndex(text); m != nil {
		text = text[:m[0]] + line + text[m[1]:]
	} else {
		end := loc[1] + 1 // logo depois da quebra de linha de biomes:
		text = text[:end] + line + text[end:]
	}
	block := kidsBlock(k)
	if start, end, ok := kidsSpan(text); ok {
		return []byte(text[:start] + block + text[end:]), nil
	}
	return []byte(text + block), nil
}

// kidsSpan acha o bloco kids: — da linha "kids:" até a próxima chave do topo
// (uma linha que começa sem espaço e não é comentário) ou o fim do arquivo.
func kidsSpan(text string) (start, end int, ok bool) {
	m := kidsLine.FindStringIndex(text)
	if m == nil {
		return 0, 0, false
	}
	pos := m[1]
	for pos < len(text) {
		lineEnd := len(text)
		if nl := strings.IndexByte(text[pos:], '\n'); nl >= 0 {
			lineEnd = pos + nl + 1
		}
		if c := text[pos]; c != ' ' && c != '\t' && c != '\n' && c != '#' {
			return m[0], pos, true
		}
		pos = lineEnd
	}
	return m[0], len(text), true
}

// kidsBlock escreve o bloco kids: no formato da spec (seção 3.1). A descrição
// usa o estilo ">" ("dobrado": na leitura, as quebras de linha viram
// espaços), em linhas de até 76 caracteres.
func kidsBlock(k domain.KidsContent) string {
	var b strings.Builder
	b.WriteString("kids:\n  description: >\n")
	for _, line := range wrap(k.Description, 76) {
		b.WriteString("    " + line + "\n")
	}
	b.WriteString("  facts:\n")
	for _, f := range k.Facts {
		fmt.Fprintf(&b, "    - text: %s\n      source_fact: %d\n", yamlScalar(f.Text), f.SourceFact)
	}
	fmt.Fprintf(&b, "  did_you_know: %s\n  generated_by: %s\n  source_hash: %s\n",
		yamlScalar(k.DidYouKnow), k.GeneratedBy, k.SourceHash)
	return b.String()
}

// yamlScalar escreve um texto de uma linha. Sem aspas, como no resto do
// catálogo, a não ser que o texto tenha algo que o YAML leria de outro jeito:
// ": " (viraria um mapa), " #" (começo de comentário) ou um caractere especial
// no começo. Aí vai entre aspas duplas, com \ e " escapados.
func yamlScalar(s string) string {
	if s == "" || strings.ContainsAny(s[:1], "-?:,[]{}#&*!|>'\"%@`") || strings.Contains(s, ": ") ||
		strings.Contains(s, " #") || strings.HasSuffix(s, ":") {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	return s
}

// wrap quebra o texto em linhas de até width caracteres, sem cortar palavras.
func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(text) {
		switch {
		case line == "":
			line = w
		case len([]rune(line))+1+len([]rune(w)) > width:
			lines = append(lines, line)
			line = w
		default:
			line += " " + w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
