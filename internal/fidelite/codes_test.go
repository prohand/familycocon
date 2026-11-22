// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"strings"
	"testing"
)

func TestTableCode128(t *testing.T) {
	vus := map[string]bool{}
	for i, m := range motifs128 {
		somme := 0
		for _, c := range m {
			somme += int(c - '0')
		}
		attendu := 11
		if i == fin128 {
			attendu = 13
		}
		if somme != attendu {
			t.Errorf("symbole %d (%s) : %d modules", i, m, somme)
		}
		if vus[m] {
			t.Errorf("symbole %d en double", i)
		}
		vus[m] = true
	}
}

// decoder128 relit les largeurs produites (aller-retour) et vérifie la clé.
func decoder128(t *testing.T, l []int) string {
	t.Helper()
	parMotif := map[string]int{}
	for i, m := range motifs128 {
		parMotif[m] = i
	}
	var vals []int
	for i := 0; i+6 <= len(l)-7; i += 6 {
		var b strings.Builder
		for _, w := range l[i : i+6] {
			b.WriteByte(byte('0' + w))
		}
		v, ok := parMotif[b.String()]
		if !ok {
			t.Fatalf("motif inconnu %s", b.String())
		}
		vals = append(vals, v)
	}
	somme := vals[0]
	for i, v := range vals[1 : len(vals)-1] {
		somme += (i + 1) * v
	}
	if somme%103 != vals[len(vals)-1] {
		t.Fatal("clé de contrôle fausse")
	}
	var s strings.Builder
	for _, v := range vals[1 : len(vals)-1] {
		if vals[0] == debutC {
			s.WriteString(string(rune('0'+v/10)) + string(rune('0'+v%10)))
		} else {
			s.WriteRune(rune(v + 32))
		}
	}
	return s.String()
}

func TestCode128(t *testing.T) {
	for _, s := range []string{"FC-2026-0042", "123456", "ab cd", "1234567"} {
		l, err := Code128(s)
		if err != nil {
			t.Fatal(err)
		}
		if got := decoder128(t, l); got != s {
			t.Errorf("%q relu en %q", s, got)
		}
	}
	// « 123456 » : jeu C (3 symboles de données) ; clé connue : 105+12+68+168 = 353 → 44.
	l, _ := Code128("123456")
	if len(l) != 6*(1+3+1)+7 {
		t.Fatalf("jeu C non utilisé : %d largeurs", len(l))
	}
	if _, err := Code128("é"); err == nil {
		t.Fatal("caractère non ASCII accepté")
	}
}

func TestEAN13(t *testing.T) {
	// Exemples réels (clé de contrôle connue).
	for _, s := range []string{"4006381333931", "3017620422003", "5901234123457"} {
		if !EstEAN13(s) {
			t.Errorf("%s refusé", s)
		}
		m, err := EAN13(s)
		if err != nil || len(m) != 95 || !strings.HasPrefix(m, "101") || m[45:50] != "01010" {
			t.Errorf("%s : %d modules", s, len(m))
		}
	}
	if EstEAN13("4006381333932") || EstEAN13("400638133393") {
		t.Fatal("EAN faux accepté")
	}
	// 5901234123457 : chiffre 5 → parité LGGLLG ; premier chiffre codé (9) en L.
	m, _ := EAN13("5901234123457")
	if m[3:10] != eanL[9] || m[10:17] != eanG[0] {
		t.Fatal("parité")
	}
	if FormatAuto("3017620422003") != "ean13" || FormatAuto("CARTE-42") != "code128" {
		t.Fatal("format automatique")
	}
	svg, _ := SVG("3017620422003", "ean13")
	if !strings.Contains(string(svg), `class="barre"`) || strings.Contains(string(svg), "style=") {
		t.Fatal("SVG")
	}
}
