// Testes do filtro de anotações: cada par (termo, valor) da tabela da spec
// descarta a observação, e o voto negativo da comunidade é ignorado.

package inaturalist

import "testing"

func TestRejectedByAnnotation(t *testing.T) {
	cases := []struct {
		name string
		ann  annotation
		want bool
	}{
		{"morto", annotation{AttributeID: 17, ValueID: 19, VoteScore: 1}, true},
		{"vivo", annotation{AttributeID: 17, ValueID: 18, VoteScore: 3}, false},
		{"ovo (estágio de vida)", annotation{AttributeID: 1, ValueID: 7, VoteScore: 0}, true},
		{"adulto", annotation{AttributeID: 1, ValueID: 2, VoteScore: 2}, false},
		{"pena", annotation{AttributeID: 22, ValueID: 23, VoteScore: 1}, true},
		{"osso", annotation{AttributeID: 22, ValueID: 27, VoteScore: 1}, true},
		{"muda", annotation{AttributeID: 22, ValueID: 28, VoteScore: 1}, true},
		{"ovo (evidência)", annotation{AttributeID: 22, ValueID: 30, VoteScore: 1}, true},
		{"construção (ninho)", annotation{AttributeID: 22, ValueID: 35, VoteScore: 1}, true},
		{"organismo (evidência)", annotation{AttributeID: 22, ValueID: 24, VoteScore: 1}, false},
		{"pena com voto negativo", annotation{AttributeID: 22, ValueID: 23, VoteScore: -1}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rejectedByAnnotation([]annotation{c.ann}); got != c.want {
				t.Fatalf("rejectedByAnnotation(%+v) = %v, esperava %v", c.ann, got, c.want)
			}
		})
	}
	if rejectedByAnnotation(nil) {
		t.Fatal("sem anotação, a observação fica")
	}
}
