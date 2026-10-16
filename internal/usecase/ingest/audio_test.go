// Testes do caso de uso de cantos, com fonte, downloader, conversor,
// storage e repositório falsos.

package ingest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

// sabiaAudio é o job de canto usado nos testes.
var sabiaAudio = ingest.Job{ID: 2, SpeciesID: "turdus-rufiventris", ScientificName: "Turdus rufiventris", Source: ingest.SourceXenoCanto}

// ccBYNCSA é uma licença aceita, no formato de URL que o xeno-canto usa.
const ccBYNCSA = "https://creativecommons.org/licenses/by-nc-sa/4.0/"

// rec cria uma gravação candidata com id, qualidade, tipo e duração.
func rec(id, q, typ string, ms int) ingest.AudioCandidate {
	return ingest.AudioCandidate{SourceID: id, URL: "https://xeno-canto.org/" + id + "/download", PageURL: "https://xeno-canto.org/" + id,
		License: ccBYNCSA, Author: "Gravador", Quality: q, Type: typ, DurationMs: ms}
}

// audioFiles monta o "servidor" do downloader falso: URL → bytes.
func audioFiles(cands ...ingest.AudioCandidate) map[string][]byte {
	m := map[string][]byte{}
	for _, c := range cands {
		m[c.URL] = []byte(c.URL)
	}
	return m
}

// newAudio monta o caso de uso com os fakes e devolve os fakes para o teste
// conferir.
func newAudio(cands []ingest.AudioCandidate, proc fakeAudio) (ingest.IngestAudio, *fakeDownloader, *fakeStore, *fakeRepo) {
	dl, store, repo := &fakeDownloader{files: audioFiles(cands...)}, newFakeStore(), &fakeRepo{}
	return ingest.IngestAudio{Source: fakeAudioSource{cands: cands}, Downloader: dl, Audio: proc, Store: store, Repo: repo,
		Policy: domain.LicensePolicy{AllowNC: true}, MaxDuration: 30 * time.Second}, dl, store, repo
}

// Entre várias candidatas, fica a melhor pela ordem: qualidade, canto antes
// de chamado, duração boa e, por fim, a mais curta.
func TestIngestAudio_PicksBestRecording(t *testing.T) {
	cands := []ingest.AudioCandidate{rec("1", "B", "song", 20000), rec("2", "A", "call", 20000), rec("3", "A", "song", 40000),
		rec("4", "A", "song", 20000), rec("5", "D", "song", 20000)}
	uc, dl, store, repo := newAudio(cands, fakeAudio{})
	if err := uc.Run(context.Background(), sabiaAudio); err != nil {
		t.Fatal(err)
	}
	if len(dl.requested) != 1 || dl.requested[0] != "https://xeno-canto.org/4/download" {
		t.Fatalf("deveria escolher a gravação 4 (A song 20 s): %v", dl.requested)
	}
	if store.objects["species/turdus-rufiventris/audio-4.aac"] != "audio/aac" {
		t.Fatalf("objeto de áudio ausente: %v", store.objects)
	}
	if repo.audio == nil || repo.audio.DurationMs != 20000 || repo.audio.Credit.Source != "xeno-canto" ||
		repo.audio.Credit.License != "CC-BY-NC-SA" || repo.audio.Credit.SourceURL != "https://xeno-canto.org/4" {
		t.Fatalf("áudio gravado: %+v", repo.audio)
	}
}

// A duração gravada nunca passa do corte (MaxDuration).
func TestIngestAudio_CapsDuration(t *testing.T) {
	uc, _, _, repo := newAudio([]ingest.AudioCandidate{rec("1", "A", "song", 90000)}, fakeAudio{})
	if err := uc.Run(context.Background(), sabiaAudio); err != nil {
		t.Fatal(err)
	}
	if repo.audio.DurationMs != 30000 {
		t.Fatalf("duração deveria ser limitada a 30000, veio %d", repo.audio.DurationMs)
	}
}

// Sem nenhuma gravação aceitável, o job termina sem erro e o canto da
// espécie fica vazio.
func TestIngestAudio_NoRecordingsIsNotAnError(t *testing.T) {
	uc, dl, _, repo := newAudio(nil, fakeAudio{})
	if err := uc.Run(context.Background(), sabiaAudio); err != nil {
		t.Fatal(err)
	}
	if repo.audioCalls != 1 || repo.audio != nil || len(dl.requested) != 0 {
		t.Fatalf("esperava ReplaceAudio(nil) e nenhum download: calls=%d audio=%v dl=%v", repo.audioCalls, repo.audio, dl.requested)
	}
}

// Se a melhor candidata falha na conversão, a próxima é usada.
func TestIngestAudio_FallsBackToNextCandidate(t *testing.T) {
	best, next := rec("1", "A", "song", 20000), rec("2", "B", "song", 20000)
	uc, _, _, repo := newAudio([]ingest.AudioCandidate{best, next}, fakeAudio{failFor: map[string]bool{best.URL: true}})
	if err := uc.Run(context.Background(), sabiaAudio); err != nil {
		t.Fatal(err)
	}
	if repo.audio == nil || repo.audio.Credit.SourceURL != "https://xeno-canto.org/2" {
		t.Fatalf("deveria usar a segunda: %+v", repo.audio)
	}
}

// Se todas falham, o job devolve erro (e será tentado de novo).
func TestIngestAudio_AllCandidatesFailIsError(t *testing.T) {
	c := rec("1", "A", "song", 20000)
	uc, _, _, repo := newAudio([]ingest.AudioCandidate{c}, fakeAudio{failFor: map[string]bool{c.URL: true}})
	if err := uc.Run(context.Background(), sabiaAudio); err == nil || repo.audioCalls != 0 {
		t.Fatalf("falha total deve dar erro sem apagar o canto existente: %v calls=%d", err, repo.audioCalls)
	}
}

// Licença ND (sem derivados) é recusada, porque cortamos o áudio.
func TestIngestAudio_RejectsNDLicense(t *testing.T) {
	c := rec("1", "A", "song", 20000)
	c.License = "https://creativecommons.org/licenses/by-nc-nd/4.0/"
	uc, dl, _, repo := newAudio([]ingest.AudioCandidate{c}, fakeAudio{})
	if err := uc.Run(context.Background(), sabiaAudio); err != nil || len(dl.requested) != 0 || repo.audioCalls != 1 || repo.audio != nil {
		t.Fatalf("ND não pode ser baixada: err=%v dl=%v", err, dl.requested)
	}
}

// Erro da fonte vira erro do job.
func TestIngestAudio_SourceErrorIsError(t *testing.T) {
	uc := ingest.IngestAudio{Source: fakeAudioSource{err: errors.New("fora do ar")}, Repo: &fakeRepo{}}
	if err := uc.Run(context.Background(), sabiaAudio); err == nil {
		t.Fatal("erro da fonte deveria propagar")
	}
}

// Trocar o canto apaga o arquivo antigo de canto e não toca nas fotos.
func TestIngestAudio_DeletesOrphanAudioButKeepsPhotos(t *testing.T) {
	uc, _, store, _ := newAudio([]ingest.AudioCandidate{rec("4", "A", "song", 20000)}, fakeAudio{})
	store.objects["species/turdus-rufiventris/audio-OLD.aac"] = "x"
	store.objects["species/turdus-rufiventris/photo-1-thumb.webp"] = "x"
	if err := uc.Run(context.Background(), sabiaAudio); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.objects["species/turdus-rufiventris/audio-OLD.aac"]; ok {
		t.Fatal("canto antigo órfão deveria ser apagado")
	}
	if _, ok := store.objects["species/turdus-rufiventris/photo-1-thumb.webp"]; !ok {
		t.Fatal("o job de canto não pode apagar fotos")
	}
	if _, ok := store.objects["species/turdus-rufiventris/audio-4.aac"]; !ok {
		t.Fatal("o canto novo não pode ser apagado")
	}
}

// Sem gravação aceitável, o canto antigo é removido do banco e do storage.
func TestIngestAudio_NoRecordingsRemovesOldAudio(t *testing.T) {
	uc, _, store, _ := newAudio(nil, fakeAudio{})
	store.objects["species/turdus-rufiventris/audio-OLD.aac"] = "x"
	if err := uc.Run(context.Background(), sabiaAudio); err != nil {
		t.Fatal(err)
	}
	if len(store.objects) != 0 {
		t.Fatalf("sem canto aceito, o arquivo antigo deve sair: %v", store.objects)
	}
}

// Um id da fonte com caracteres perigosos (como "/") é ignorado.
func TestIngestAudio_SkipsUnsafeSourceID(t *testing.T) {
	c := rec("1", "A", "song", 20000)
	c.SourceID = "a/b"
	uc, dl, _, repo := newAudio([]ingest.AudioCandidate{c}, fakeAudio{})
	if err := uc.Run(context.Background(), sabiaAudio); err != nil || len(dl.requested) != 0 || repo.audio != nil {
		t.Fatalf("id inseguro deveria ser descartado: %v %v", err, dl.requested)
	}
}
