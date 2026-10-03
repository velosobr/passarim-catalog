package domain

import (
	"errors"
	"strings"
)

// License é uma licença Creative Commons ACEITA pelo Passarim (ADR-0007).
type License string

const (
	LicenseCC0      License = "CC0"
	LicenseCCBY     License = "CC-BY"
	LicenseCCBYSA   License = "CC-BY-SA"
	LicenseCCBYNC   License = "CC-BY-NC"
	LicenseCCBYNCSA License = "CC-BY-NC-SA"
)

// ErrLicenseNotAccepted: a mídia não pode ser usada. ND ("sem obras
// derivadas") é recusada porque redimensionamos fotos e cortamos áudios.
var ErrLicenseNotAccepted = errors.New("licença não aceita")

// ParseLicense entende os formatos que as fontes usam:
// códigos ("cc-by-nc", "CC BY-NC") e URLs da Creative Commons.
func ParseLicense(s string) (License, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	if strings.Contains(v, "creativecommons.org") {
		if strings.Contains(v, "/publicdomain/zero/") {
			return LicenseCC0, nil
		}
		// ".../licenses/by-nc-sa/4.0/" → "by-nc-sa"
		if i := strings.Index(v, "/licenses/"); i >= 0 {
			v = strings.SplitN(v[i+len("/licenses/"):], "/", 2)[0]
		}
	}
	v = strings.ReplaceAll(v, " ", "-")
	v = strings.TrimPrefix(v, "cc-")
	switch v {
	case "cc0", "zero":
		return LicenseCC0, nil
	case "by":
		return LicenseCCBY, nil
	case "by-sa":
		return LicenseCCBYSA, nil
	case "by-nc":
		return LicenseCCBYNC, nil
	case "by-nc-sa":
		return LicenseCCBYNCSA, nil
	}
	return "", ErrLicenseNotAccepted
}

// LicensePolicy decide o que este app pode usar. Gratuito e sem anúncios:
// NC permitido. Se o app um dia for monetizado, basta AllowNC=false.
type LicensePolicy struct {
	AllowNC bool
}

func (p LicensePolicy) Accepts(l License) bool {
	switch l {
	case LicenseCC0, LicenseCCBY, LicenseCCBYSA:
		return true
	case LicenseCCBYNC, LicenseCCBYNCSA:
		return p.AllowNC
	}
	return false
}
