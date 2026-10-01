// Package curated lê o conteúdo escrito à mão (content/species/*.yaml)
// e o transforma em domain.Species. É um "adapter de entrada" de dados.
package curated

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// speciesFile espelha o formato do YAML. As tags `yaml:"..."` dizem qual
// chave do arquivo vai para qual campo.
type speciesFile struct {
	ScientificName       string     `yaml:"scientific_name"`
	CommonNamePt         string     `yaml:"common_name_pt"`
	Family               string     `yaml:"family"`
	SizeCm               *int       `yaml:"size_cm"`
	Diet                 *string    `yaml:"diet"`
	ConservationStatus   string     `yaml:"conservation_status"`
	Biomes               []string   `yaml:"biomes"`
	States               []string   `yaml:"states"`
	Description          string     `yaml:"description"`
	DescriptionSourceURL string     `yaml:"description_source_url"`
	Facts                []factFile `yaml:"facts"`
}

type factFile struct {
	Text   string `yaml:"text"`
	Source string `yaml:"source"`
}

// LoadDir lê todos os .yaml da pasta, valida cada um e devolve a lista
// ordenada por ID. Se QUALQUER arquivo tiver problema, nada é devolvido:
// melhor falhar inteiro do que gravar metade do catálogo.
func LoadDir(dir string) ([]domain.Species, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("nenhum .yaml encontrado em %s", dir)
	}

	out := make([]domain.Species, 0, len(paths))
	for _, p := range paths {
		s, err := loadFile(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func loadFile(path string) (domain.Species, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.Species{}, err
	}
	var f speciesFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	// KnownFields(true): uma chave desconhecida (ex.: "familly") vira ERRO
	// em vez de ser ignorada silenciosamente.
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return domain.Species{}, err
	}

	s := domain.Species{
		ID:                 domain.SpeciesID(f.ScientificName),
		ScientificName:     strings.Join(strings.Fields(f.ScientificName), " "),
		CommonNamePt:       strings.TrimSpace(f.CommonNamePt),
		Family:             strings.TrimSpace(f.Family),
		SizeCm:             f.SizeCm,
		Diet:               f.Diet,
		ConservationStatus: domain.ConservationStatus(f.ConservationStatus),
		Description:        strings.TrimSpace(f.Description),
		// Texto escrito pela equipe: licença CC-BY-4.0, com link de referência.
		DescriptionCredit: domain.Credit{
			Author: "Equipe Passarim", License: "CC-BY-4.0", Source: "curated", SourceURL: f.DescriptionSourceURL,
		},
		States: f.States,
	}
	for _, b := range f.Biomes {
		s.Biomes = append(s.Biomes, domain.Biome(b))
	}
	for _, fact := range f.Facts {
		s.Facts = append(s.Facts, domain.Fact{Text: strings.TrimSpace(fact.Text), Source: strings.TrimSpace(fact.Source)})
	}
	if err := s.Validate(); err != nil {
		return domain.Species{}, err
	}
	return s, nil
}
