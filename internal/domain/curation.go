// O vocabulário da curadoria de fotos (frente A) no domínio: o que uma foto
// mostra, a escolha manual do YAML (override) e o relatório de cada execução.
//
// Quem usa: o caso de uso IngestPhotos (escolhe as fotos), o seed (grava o
// override) e o comando curation-report do worker. Nada aqui sabe de gRPC,
// SQL ou JSON: os adapters traduzem.

package domain

import (
	"regexp"
	"slices"
	"time"
)

// PhotoCategory diz o que uma foto mostra, segundo o curador
// (passarim-photo-curator). Só PhotoSingleBird concorre a foto principal;
// PhotoFlock entra no fim da galeria; o resto é descarte.
type PhotoCategory string

const (
	PhotoUnspecified PhotoCategory = "unspecified" // a imagem não abriu no curador
	PhotoSingleBird  PhotoCategory = "single_bird"
	PhotoFlock       PhotoCategory = "flock"
	PhotoNest        PhotoCategory = "nest"
	PhotoEggs        PhotoCategory = "eggs"
	PhotoFeather     PhotoCategory = "feather"
	PhotoDeadBird    PhotoCategory = "dead_bird"
	PhotoNoBird      PhotoCategory = "no_bird"
	PhotoOther       PhotoCategory = "other" // ave na mão, ilustração, ave escondida
)

// PhotoOverride é a escolha manual da chave photos: do YAML de uma espécie.
// Os ids são os do iNaturalist, guardados como texto (como o SourceID das
// candidatas).
//
//   - PrimaryObservationID e PrimaryPhotoID: a foto que vira a principal, seja
//     qual for a categoria que o curador der. Vêm juntos ou não vêm.
//   - ExcludedIDs: fotos que nunca entram, mesmo se forem as melhores.
type PhotoOverride struct {
	PrimaryObservationID string
	PrimaryPhotoID       string
	ExcludedIDs          []string
}

// inatIDPattern: um id do iNaturalist é um inteiro positivo, sem zero à
// esquerda. Validar aqui evita que um id estranho chegue a uma URL ou a uma
// chave do storage.
var inatIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

// HasPrimary diz se o YAML escolheu uma foto principal.
func (o PhotoOverride) HasPrimary() bool { return o.PrimaryPhotoID != "" }

// Excludes diz se a foto está na lista de exclusões.
func (o PhotoOverride) Excludes(photoID string) bool { return slices.Contains(o.ExcludedIDs, photoID) }

// Validate recusa o override vazio, ids que não são do iNaturalist, a
// principal pela metade e a principal que também está excluída (o YAML
// estaria pedindo duas coisas opostas).
func (o PhotoOverride) Validate() error {
	if o.PrimaryObservationID == "" && o.PrimaryPhotoID == "" && len(o.ExcludedIDs) == 0 {
		return invalid("photos", "vazio: use primary, exclude ou tire a chave")
	}
	if o.PrimaryObservationID != "" || o.PrimaryPhotoID != "" {
		if !inatIDPattern.MatchString(o.PrimaryObservationID) || !inatIDPattern.MatchString(o.PrimaryPhotoID) {
			return invalid("photos.primary", "precisa de observation e photo, ids positivos do iNaturalist")
		}
	}
	for _, id := range o.ExcludedIDs {
		if !inatIDPattern.MatchString(id) {
			return invalid("photos.exclude", "id inválido: "+id)
		}
	}
	if o.HasPrimary() && o.Excludes(o.PrimaryPhotoID) {
		return invalid("photos.exclude", "a foto principal "+o.PrimaryPhotoID+" também está excluída")
	}
	return nil
}

// CurationFlag marca uma espécie no relatório para o dono conferir.
type CurationFlag string

const (
	// FlagLowConfidence: a principal escolhida pelo modelo tem confiança
	// abaixo de LOW_CONFIDENCE.
	FlagLowConfidence CurationFlag = "low_confidence"
	// FlagNoSingleBird: nenhuma candidata saiu "ave sozinha".
	FlagNoSingleBird CurationFlag = "no_single_bird"
	// FlagOverrideFailed: a principal do YAML sumiu, mudou de licença ou não
	// abriu; a principal voltou a ser a do modelo.
	FlagOverrideFailed CurationFlag = "override_failed"
)

// CuratedCandidate é uma candidata classificada, como aparece no relatório.
type CuratedCandidate struct {
	PhotoID       string
	ObservationID string
	Category      PhotoCategory
	Score         float64
	Confidence    float64
}

// CurationReport é o resultado de uma execução da curadoria de uma espécie.
// PrimaryPhotoID fica "" quando a espécie ficou sem fotos.
type CurationReport struct {
	ModelVersion   string
	PrimaryPhotoID string
	Candidates     []CuratedCandidate
	Flags          []CurationFlag
}

// CurationReportEntry é uma linha do relatório lido do banco: a última
// execução de uma espécie, com o nome para o dono reconhecer.
type CurationReportEntry struct {
	SpeciesID  string
	CommonName string
	RanAt      time.Time
	Report     CurationReport
}
