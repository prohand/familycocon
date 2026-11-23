// SPDX-License-Identifier: GPL-3.0-or-later

package texte

import "testing"

func TestPlier(t *testing.T) {
	cas := map[string]string{"ÉCOLE": "ecole", "Noël": "noel", "Hélène": "helene", "Œuf": "oeuf", "Ça": "ca", "abc 12": "abc 12"}
	for entree, attendu := range cas {
		if got := Plier(entree); got != attendu {
			t.Errorf("Plier(%q) = %q, attendu %q", entree, got, attendu)
		}
	}
	if !Contient("Anniversaire de Noël", "NOEL") || Contient("Pain", "lait") {
		t.Error("Contient")
	}
}
