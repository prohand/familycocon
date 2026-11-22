// SPDX-License-Identifier: GPL-3.0-or-later

package courses

import "testing"

func TestAnalyser(t *testing.T) {
	cas := []struct{ saisie, nom, qte string }{
		{"pain", "Pain", ""},
		{"2 baguettes", "Baguettes", "2"},
		{"1 kg de pommes", "Pommes", "1 kg"},
		{"2 l d'eau", "Eau", "2 l"},
		{"6 œufs", "Œufs", "6"},
		{"lait x3", "Lait", "3"},
		{"  beurre   doux ", "Beurre doux", ""},
		{"3", "3", ""},
		{"1,5 kg de farine", "Farine", "1,5 kg"},
	}
	for _, c := range cas {
		nom, qte := Analyser(c.saisie)
		if nom != c.nom || qte != c.qte {
			t.Errorf("%q : (%q, %q), attendu (%q, %q)", c.saisie, nom, qte, c.nom, c.qte)
		}
	}
}

func TestDeviner(t *testing.T) {
	cas := map[string]string{
		"Pommes de terre": "fruits", "Pommes": "fruits", "Baguettes": "boulangerie", "Œufs": "frais",
		"Jus d'orange bio": "boissons", "Papier toilette": "hygiene", "Croquettes chat": "animaux",
		"Truc inconnu": "autre", "Gruyère râpé": "frais", "Choux de Bruxelles": "fruits",
	}
	for nom, attendu := range cas {
		if r, _ := Deviner(nom); r != attendu {
			t.Errorf("%q : %s, attendu %s", nom, r, attendu)
		}
	}
	if _, e := Deviner("Pomme de terre"); e != "🥔" {
		t.Errorf("emoji pomme de terre : %s", e)
	}
	for _, r := range dictionnaire {
		if _, ok := RayonParCle(r.rayon); !ok {
			t.Errorf("rayon inconnu dans le dictionnaire : %s", r.rayon)
		}
	}
}
