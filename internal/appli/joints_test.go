// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"strings"
	"testing"
)

func TestNomJointEtTaille(t *testing.T) {
	for v, attendu := range map[string]string{
		"facture.pdf":                     "facture.pdf",
		"../../etc/passwd.txt":            "passwd.txt",
		`C:\Users\moi\devis.docx`:         "devis.docx",
		"  a\x00b\"c.pdf ":                "abc.pdf",
		"":                                "",
		strings.Repeat("é", 200) + ".pdf": strings.Repeat("é", nomJointMax-4) + ".pdf",
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
	for v, attendu := range map[string]string{"/notes/3": "/notes/3", "//pirate.fr": "/", "https://x": "/", `/\pirate`: "/"} {
		if r := retourSur(v); r != attendu {
			t.Errorf("retourSur(%q) = %q", v, r)
		}
	}
}
