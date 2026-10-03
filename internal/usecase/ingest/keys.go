package ingest

import "fmt"

// Chaves no object storage. O BFF (Etapa 3) monta a URL pública a partir delas.
func photoKey(speciesID string, n int, variant string) string {
	return fmt.Sprintf("species/%s/photo-%d-%s.webp", speciesID, n, variant)
}

func audioKey(speciesID string) string {
	return fmt.Sprintf("species/%s/audio-0.aac", speciesID)
}
