// Package domain contém as regras de negócio puras do catálogo de aves.
// Nada aqui conhece banco de dados, gRPC ou HTTP — só Go puro.
// Por isso dá para testar tudo sem subir nenhum container.
package domain

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Biome é um dos seis biomas brasileiros.
type Biome string

const (
	BiomeAmazonia      Biome = "amazonia"
	BiomeMataAtlantica Biome = "mata_atlantica"
	BiomeCerrado       Biome = "cerrado"
	BiomeCaatinga      Biome = "caatinga"
	BiomePantanal      Biome = "pantanal"
	BiomePampa         Biome = "pampa"
)

var allBiomes = []Biome{BiomeAmazonia, BiomeMataAtlantica, BiomeCerrado, BiomeCaatinga, BiomePantanal, BiomePampa}

// AllBiomes devolve uma CÓPIA da lista (quem chamar não consegue alterar a original).
func AllBiomes() []Biome { return append([]Biome(nil), allBiomes...) }

// ParseBiome converte texto em Biome, recusando valores desconhecidos.
func ParseBiome(s string) (Biome, error) {
	for _, b := range allBiomes {
		if string(b) == s {
			return b, nil
		}
	}
	return "", invalid("biome", fmt.Sprintf("bioma desconhecido %q", s))
}

// ConservationStatus segue a Lista Vermelha da IUCN. Vazio = ainda não sabemos.
type ConservationStatus string

const (
	StatusUnknown ConservationStatus = ""
	StatusLC      ConservationStatus = "LC" // pouco preocupante
	StatusNT      ConservationStatus = "NT" // quase ameaçada
	StatusVU      ConservationStatus = "VU" // vulnerável
	StatusEN      ConservationStatus = "EN" // em perigo
	StatusCR      ConservationStatus = "CR" // criticamente em perigo
	StatusEW      ConservationStatus = "EW" // extinta na natureza
	StatusEX      ConservationStatus = "EX" // extinta
	StatusDD      ConservationStatus = "DD" // dados insuficientes
)

// ParseConservationStatus aceita apenas as categorias da IUCN (ou vazio).
func ParseConservationStatus(s string) (ConservationStatus, error) {
	switch c := ConservationStatus(s); c {
	case StatusUnknown, StatusLC, StatusNT, StatusVU, StatusEN, StatusCR, StatusEW, StatusEX, StatusDD:
		return c, nil
	}
	return "", invalid("conservation_status", fmt.Sprintf("categoria IUCN desconhecida %q", s))
}

// As 27 unidades federativas: 26 estados + Distrito Federal.
var validUFs = map[string]bool{
	"AC": true, "AL": true, "AP": true, "AM": true, "BA": true, "CE": true, "DF": true,
	"ES": true, "GO": true, "MA": true, "MT": true, "MS": true, "MG": true, "PA": true,
	"PB": true, "PR": true, "PE": true, "PI": true, "RJ": true, "RN": true, "RS": true,
	"RO": true, "RR": true, "SC": true, "SP": true, "SE": true, "TO": true,
}

// AllUFs devolve as siglas em ordem alfabética.
func AllUFs() []string {
	ufs := make([]string, 0, len(validUFs))
	for uf := range validUFs {
		ufs = append(ufs, uf)
	}
	sort.Strings(ufs)
	return ufs
}

// NormalizeUF aceita " ms " e devolve "MS"; recusa siglas que não existem.
func NormalizeUF(s string) (string, error) {
	uf := strings.ToUpper(strings.TrimSpace(s))
	if !validUFs[uf] {
		return "", invalid("state", fmt.Sprintf("UF desconhecida %q", s))
	}
	return uf, nil
}

// SpeciesID cria o identificador estável de uma espécie a partir do nome
// científico: "Turdus rufiventris" -> "turdus-rufiventris". Usamos o nome
// científico porque o popular muda de região para região.
func SpeciesID(scientificName string) string {
	return strings.ToLower(strings.Join(strings.Fields(scientificName), "-"))
}

// accentRemover decompõe "á" em "a" + acento (NFD), remove os acentos
// (categoria Unicode Mn = "marcas não espaçadas") e recompõe (NFC).
var accentRemover = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// NormalizeForSearch deixa o texto sem acentos e em minúsculas, para que
// "sabia" encontre "Sabiá". É usada tanto ao GRAVAR quanto ao BUSCAR:
// as duas pontas precisam normalizar do mesmo jeito.
func NormalizeForSearch(s string) string {
	out, _, err := transform.String(accentRemover, s)
	if err != nil {
		out = s // em caso de texto Unicode inválido, segue sem remover acentos
	}
	return strings.ToLower(strings.TrimSpace(out))
}

// NormalizeSearchText vai além de NormalizeForSearch: também troca hífen,
// pontuação e espaços repetidos por UM espaço só. É usada SÓ na busca (a
// coluna search_text e a query do usuário), nunca no sort_name — assim
// "bem te vi" e "joao de barro" encontram "Bem-te-vi" e "João-de-barro".
func NormalizeSearchText(s string) string {
	base := NormalizeForSearch(s)
	var b strings.Builder
	prevSpace := true // começa "true" para já descartar espaços no início
	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Credit guarda a autoria exigida pelas licenças Creative Commons.
type Credit struct {
	Author    string
	License   string
	Source    string
	SourceURL string
}

// Fact é uma curiosidade com a fonte de onde ela veio.
type Fact struct {
	Text   string
	Source string
}

// Photo guarda CHAVES do object storage, não URLs (quem monta a URL é o BFF).
type Photo struct {
	ThumbKey, MediumKey, LargeKey string
	Width, Height                 int
	Credit                        Credit
}

// Audio é a gravação do canto.
type Audio struct {
	Key        string
	DurationMs int
	Credit     Credit
}

// OccurrenceCluster agrupa avistamentos próximos num único ponto do mapa.
type OccurrenceCluster struct {
	Lat, Lng  float64
	Count     int
	Precision float64
}

// SpeciesSummary é a versão leve, usada nos cards da lista.
type SpeciesSummary struct {
	ID                 string
	ScientificName     string
	CommonNamePt       string
	ThumbnailKey       string
	ConservationStatus ConservationStatus
	// SortName é o sort_name GRAVADO no banco (não recalculado). O cursor de
	// paginação precisa encodar exatamente o valor com o qual o SQL comparou
	// (ORDER BY sort_name, id) — se a aplicação recalculasse a partir de
	// CommonNamePt e o valor gravado tivesse ficado diferente por qualquer
	// motivo, a página 2 repetiria ou pularia itens (Review Focus #2).
	SortName string
}

// Species é a versão completa, usada na tela de detalhe.
// Ponteiros (*int, *string, *Audio) significam "pode não existir":
// nil é diferente de zero (tamanho desconhecido ≠ 0 cm).
type Species struct {
	ID                 string
	ScientificName     string
	CommonNamePt       string
	Family             string
	SizeCm             *int
	Diet               *string
	ConservationStatus ConservationStatus
	Description        string
	DescriptionCredit  Credit
	Facts              []Fact
	Biomes             []Biome
	States             []string
	Photos             []Photo
	Audio              *Audio
	Clusters           []OccurrenceCluster
}

// Validate verifica as regras que toda espécie precisa cumprir antes
// de ser gravada. Devolve o PRIMEIRO problema encontrado.
func (s Species) Validate() error {
	if len(strings.Fields(s.ScientificName)) != 2 {
		return invalid("scientific_name", "deve ter gênero e espécie, ex.: \"Turdus rufiventris\"")
	}
	if s.ID != SpeciesID(s.ScientificName) {
		return invalid("id", fmt.Sprintf("deve ser %q", SpeciesID(s.ScientificName)))
	}
	if strings.TrimSpace(s.CommonNamePt) == "" {
		return invalid("common_name_pt", "obrigatório")
	}
	if strings.TrimSpace(s.Family) == "" {
		return invalid("family", "obrigatório")
	}
	if s.SizeCm != nil && *s.SizeCm <= 0 {
		return invalid("size_cm", "deve ser maior que zero")
	}
	if _, err := ParseConservationStatus(string(s.ConservationStatus)); err != nil {
		return err
	}
	for _, b := range s.Biomes {
		if _, err := ParseBiome(string(b)); err != nil {
			return invalid("biomes", err.Error())
		}
	}
	for _, uf := range s.States {
		// Exigimos a forma já normalizada ("SP"), para o banco ficar consistente.
		if n, err := NormalizeUF(uf); err != nil || n != uf {
			return invalid("states", fmt.Sprintf("UF inválida %q (use maiúsculas, ex.: SP)", uf))
		}
	}
	for i, f := range s.Facts {
		if strings.TrimSpace(f.Text) == "" || strings.TrimSpace(f.Source) == "" {
			return invalid("facts", fmt.Sprintf("curiosidade %d precisa de texto e fonte", i+1))
		}
	}
	return nil
}
