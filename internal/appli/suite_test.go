// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import "testing"

// Adresse de retour après connexion : jamais vers un autre site.
func TestSuiteSure(t *testing.T) {
	refusees := []string{"", "https://evil.example", "//evil.example", "/\\evil.example", "/\t/evil.example",
		"/\n/evil.example", "/\r//evil.example", "/%09/evil.example\x00", "/connexion", "\\\\evil.example", "/a\\b"}
	for _, s := range refusees {
		if suiteSure(s) != "/" {
			t.Errorf("%q accepté", s)
		}
	}
	for _, s := range []string{"/agenda", "/courses?liste=2", "/contacts/3"} {
		if suiteSure(s) != s {
			t.Errorf("%q refusé", s)
		}
	}
}
