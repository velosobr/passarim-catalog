// Testes das licenças.

package domain_test

import (
	"errors"
	"testing"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

// Os vários formatos que as fontes usam (código, com espaço, URL da
// Creative Commons) viram a mesma licença; ND e textos desconhecidos são
// recusados.
func TestParseLicense(t *testing.T) {
	ok := map[string]domain.License{
		"cc0":         domain.LicenseCC0,
		"cc-by":       domain.LicenseCCBY,
		"CC BY-SA":    domain.LicenseCCBYSA,
		"cc-by-nc":    domain.LicenseCCBYNC,
		"cc-by-nc-sa": domain.LicenseCCBYNCSA,
		"https://creativecommons.org/licenses/by-nc-sa/4.0/": domain.LicenseCCBYNCSA,
		"//creativecommons.org/licenses/by/4.0/":             domain.LicenseCCBY,
		"https://creativecommons.org/publicdomain/zero/1.0/": domain.LicenseCC0,
	}
	for in, want := range ok {
		got, err := domain.ParseLicense(in)
		if err != nil || got != want {
			t.Errorf("ParseLicense(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	// ND (sem obras derivadas), vazio ("todos os direitos
	// reservados") e lixo são recusados.
	for _, in := range []string{"cc-by-nd", "cc-by-nc-nd", "https://creativecommons.org/licenses/by-nc-nd/4.0/", "", "all rights reserved", "gpl"} {
		if _, err := domain.ParseLicense(in); !errors.Is(err, domain.ErrLicenseNotAccepted) {
			t.Errorf("ParseLicense(%q) deveria recusar, veio %v", in, err)
		}
	}
}

// Com AllowNC desligado, as licenças NC deixam de ser aceitas.
func TestLicensePolicy(t *testing.T) {
	strict := domain.LicensePolicy{AllowNC: false}
	if strict.Accepts(domain.LicenseCCBYNC) || !strict.Accepts(domain.LicenseCCBY) {
		t.Fatal("sem NC: deveria recusar CC-BY-NC e aceitar CC-BY")
	}
	if !(domain.LicensePolicy{AllowNC: true}).Accepts(domain.LicenseCCBYNCSA) {
		t.Fatal("com NC: deveria aceitar CC-BY-NC-SA")
	}
}
