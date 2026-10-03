package ingest

import "testing"

func TestStorageKeys(t *testing.T) {
	if got := photoKey("turdus-rufiventris", "98729447", "medium"); got != "species/turdus-rufiventris/photo-98729447-medium.webp" {
		t.Errorf("photoKey = %q", got)
	}
	if got := audioKey("turdus-rufiventris", "1172942"); got != "species/turdus-rufiventris/audio-1172942.aac" {
		t.Errorf("audioKey = %q", got)
	}
}

func TestValidSourceID(t *testing.T) {
	for _, ok := range []string{"98729447", "XC123", "a-b_c"} {
		if !validSourceID(ok) {
			t.Errorf("%q deveria ser válido", ok)
		}
	}
	// O id vem de fora e vira parte de uma chave de storage: nada de "/", ".." ou vazio.
	for _, bad := range []string{"", "../x", "a/b", "a b", "x.webp", "é"} {
		if validSourceID(bad) {
			t.Errorf("%q deveria ser recusado", bad)
		}
	}
}
