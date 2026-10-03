package ingest

import "testing"

func TestStorageKeys(t *testing.T) {
	if got := photoKey("turdus-rufiventris", 2, "medium"); got != "species/turdus-rufiventris/photo-2-medium.webp" {
		t.Errorf("photoKey = %q", got)
	}
	if got := audioKey("turdus-rufiventris"); got != "species/turdus-rufiventris/audio-0.aac" {
		t.Errorf("audioKey = %q", got)
	}
}
