// Testes da leitura dos argumentos do comando requeue.

package main

import (
	"fmt"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/usecase/ingest"
)

func TestParseRequeueArgs(t *testing.T) {
	cases := []struct {
		args         []string
		source       ingest.Source
		ids, wantErr string
	}{
		{nil, ingest.SourceINaturalist, "[]", ""},
		{[]string{"crotophaga-ani"}, ingest.SourceINaturalist, "[crotophaga-ani]", ""},
		{[]string{"--source", "audio"}, ingest.SourceXenoCanto, "[]", ""},
		{[]string{"--source=photos", "a", "b"}, ingest.SourceINaturalist, "[a b]", ""},
		{[]string{"--source", "video"}, "", "", "--source deve ser photos ou audio"},
	}
	for _, tc := range cases {
		source, ids, err := parseRequeueArgs(tc.args)
		if tc.wantErr != "" {
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("%v: esperava o erro %q, veio %v", tc.args, tc.wantErr, err)
			}
			continue
		}
		if err != nil || source != tc.source || fmt.Sprint(ids) != tc.ids {
			t.Fatalf("%v: fonte=%q ids=%v err=%v", tc.args, source, ids, err)
		}
	}
}
