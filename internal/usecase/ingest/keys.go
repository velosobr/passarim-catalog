// Chaves dos arquivos de mídia no object storage e a limpeza dos arquivos
// que ninguém usa mais.

package ingest

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Chaves no object storage. O BFF monta a URL pública a partir delas.
//
// A chave leva o ID DA FOTO/GRAVAÇÃO NA FONTE, não a posição. Assim a mesma
// foto sempre tem a mesma chave e uma foto diferente nunca sobrescreve a
// anterior: se um refresh falhar no meio, o crédito no banco continua
// apontando para o arquivo certo. O conteúdo de uma chave também nunca muda,
// o que permite à CDN guardá-lo em cache para sempre.
func photoKey(speciesID, sourceID, variant string) string {
	return fmt.Sprintf("species/%s/photo-%s-%s.webp", speciesID, sourceID, variant)
}

// audioKey é a chave de um canto.
func audioKey(speciesID, sourceID string) string {
	return fmt.Sprintf("species/%s/audio-%s.aac", speciesID, sourceID)
}

// photoPrefix e audioPrefix são o começo comum das chaves de uma espécie,
// usado para listar o que há no storage.
func photoPrefix(speciesID string) string { return fmt.Sprintf("species/%s/photo-", speciesID) }
func audioPrefix(speciesID string) string { return fmt.Sprintf("species/%s/audio-", speciesID) }

// sourceIDPattern é a expressão regular dos ids aceitos: até 64 letras,
// números, "_" ou "-".
var sourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// validSourceID: o id vem de uma API externa e vira parte de uma chave de
// storage, então aceitamos só letras, números, "_" e "-" (nada de "/" ou "..").
func validSourceID(id string) bool { return sourceIDPattern.MatchString(id) }

// removeOrphans apaga do storage os objetos sob prefix que NÃO estão em keep.
// Roda DEPOIS de o banco já apontar para os arquivos novos. É "melhor esforço":
// se falhar, o banco continua correto e o próximo refresh tenta de novo.
func removeOrphans(ctx context.Context, store MediaStore, prefix string, keep map[string]bool) {
	existing, err := store.List(ctx, prefix)
	if err != nil {
		return
	}
	var orphans []string
	for _, k := range existing {
		if strings.HasPrefix(k, prefix) && !keep[k] {
			orphans = append(orphans, k)
		}
	}
	if len(orphans) > 0 {
		_ = store.Delete(ctx, orphans)
	}
}
