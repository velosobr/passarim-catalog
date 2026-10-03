package xenocanto

import "testing"

func TestParseLength(t *testing.T) {
	cases := map[string]int{"1:26": 86000, "0:07": 7000, "lixo": 0, "": 0, "a:b": 0, "-1:00": 0}
	for in, want := range cases {
		if got := parseLength(in); got != want {
			t.Errorf("parseLength(%q) = %d, want %d", in, got, want)
		}
	}
}
