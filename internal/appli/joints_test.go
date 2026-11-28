// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNomJointEtTaille(t *testing.T) {
	for v, attendu := range map[string]string{
		"facture.pdf":                     "facture.pdf",
		"../../etc/passwd.txt":            "passwd.txt",
		`C:\Users\moi\devis.docx`:         "devis.docx",
		"  a\x00b\"c.pdf ":                "abc.pdf",
		"":                                "",
		strings.Repeat("é", 200) + ".pdf": strings.Repeat("é", nomJointMax-4) + ".pdf",
		"a." + strings.Repeat("x", 200):   "a." + strings.Repeat("x", nomJointMax-2), // extension sans fin : pas de plantage
	} {
		if r := nomJoint(v); r != attendu {
			t.Errorf("nomJoint(%q) = %q, attendu %q", v, r, attendu)
		}
	}
	for n, attendu := range map[int64]string{500: "500 o", 2048: "2 Ko", 1572864: "1,5 Mo"} {
		if r := TailleLisible(n); r != attendu {
			t.Errorf("TailleLisible(%d) = %q, attendu %q", n, r, attendu)
		}
	}
	for v, attendu := range map[string]string{"/notes/3": "/notes/3", "//pirate.fr": "/", "https://x": "/", `/\pirate`: "/", "/\t/pirate.fr": "/", "/\n/pirate.fr": "/"} {
		if r := retourSur(v); r != attendu {
			t.Errorf("retourSur(%q) = %q", v, r)
		}
	}
}

func TestDateCourte(t *testing.T) {
	maintenant := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	if r := DateCourte(time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC), maintenant); r != "10/12" {
		t.Errorf("même année : %q", r)
	}
	if r := DateCourte(time.Date(2027, 1, 14, 0, 0, 0, 0, time.UTC), maintenant); r != "14/01/2027" {
		t.Errorf("année suivante : %q", r)
	}
}

// Fichiers provisoires d'un arrêt brutal : effacés au démarrage, sans
// toucher aux vrais fichiers.
func TestEffacerProvisoires(t *testing.T) {
	a := nouvelleAppTest(t, map[string]string{"FC_URL_PUBLIQUE": "http://localhost:8080"})
	dossier := filepath.Join(a.DossierPieces(), "1")
	_ = os.MkdirAll(dossier, 0o750)
	provisoires := []string{filepath.Join(a.Config.Donnees, "fuelio-123.tmp"), filepath.Join(dossier, "abc.pdf.tmp")}
	gardes := []string{filepath.Join(dossier, "abc.pdf"), filepath.Join(a.Config.Donnees, "familycocon.db")}
	for _, f := range append(provisoires, gardes[0]) {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a.EffacerProvisoires()
	for _, f := range provisoires {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s pas effacé", f)
		}
	}
	for _, f := range gardes {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s effacé : %v", f, err)
		}
	}
}
