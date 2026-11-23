// SPDX-License-Identifier: GPL-3.0-or-later

package contacts

import (
	"strings"
	"testing"

	"familycocon/internal/essai"
)

// Doublon signalé à la création ; fusion : la fiche gardée est complétée,
// l'autre disparaît ; seuls les parents fusionnent.
func TestDoublonsEtFusion(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Get("/contacts/nouveau")
	_, page := marie.Post("/contacts", map[string]string{"nom": "Hélène Dupont", "telephone": "06 12 34 56 78"})
	garde := essai.Trouver(t, page, `action="/contacts/(\d+)/favori"`)
	_, page = marie.Post("/contacts", map[string]string{"nom": "helene dupont", "email": "helene@exemple.fr", "groupes": "École"})
	if !strings.Contains(page, "existe déjà") || !strings.Contains(page, "Doublon possible") {
		t.Fatalf("doublon non signalé :\n%s", page)
	}
	autre := essai.Trouver(t, page, `action="/contacts/(\d+)/favori"`)
	if code, _ := lea.Post("/contacts/"+garde+"/fusionner", map[string]string{"autre": autre}); code != 403 {
		t.Fatalf("fusion par un enfant : %d", code)
	}
	if code, _ := marie.Post("/contacts/"+garde+"/fusionner", map[string]string{"autre": autre}); code != 200 {
		t.Fatalf("fusion : %d", code)
	}
	_, page = marie.Get("/contacts/" + garde)
	if !strings.Contains(page, "helene@exemple.fr") || !strings.Contains(page, "École") || strings.Contains(page, "Doublon possible") {
		t.Fatalf("fiche fusionnée :\n%s", page)
	}
	if code, _ := marie.Get("/contacts/" + autre); code != 404 {
		t.Fatal("l'autre fiche existe encore")
	}
}
