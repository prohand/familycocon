// SPDX-License-Identifier: GPL-3.0-or-later

package projets

import (
	"net/http"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

// Modifier une étape (texte, date, qui) : tous ceux qui voient le projet ;
// projet privé : son propriétaire seul.
func TestModifierEtape(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")

	marie.Get("/projets/nouveau")
	_, page := marie.Post("/projets", map[string]string{"nom": "Déménagement"})
	id := essai.Trouver(t, page, `action="/projets/(\d+)/etapes"`)
	_, page = marie.Post("/projets/"+id+"/etapes", map[string]string{"texte": "Réserver le camion"})
	etape := essai.Trouver(t, page, `action="/projets/etapes/(\d+)/modifier"`)
	idLea := essai.Trouver(t, page, `<option value="(\d+)">Léa</option>`)
	if !strings.Contains(page, `<summary class="quoi" title="Modifier l'étape">Réserver le camion`) {
		t.Fatalf("étape pas modifiable en la touchant :\n%s", page)
	}

	// Léa (elle voit le projet) change le texte, la date et qui s'en occupe.
	demain := essai.Maintenant().AddDate(0, 0, 1).Format("2006-01-02")
	lea.Get("/projets/" + id)
	code, page := lea.Post("/projets/etapes/"+etape+"/modifier", map[string]string{"texte": "Réserver le grand camion", "jour": demain, "membre": idLea})
	if code != 200 || !strings.Contains(page, "Réserver le grand camion") || !strings.Contains(page, `value="`+demain+`"`) ||
		!strings.Contains(page, `<option value="`+idLea+`" selected>Léa</option>`) {
		t.Fatalf("modification : %d\n%s", code, page)
	}
	if code, _ := lea.Post("/projets/etapes/"+etape+"/modifier", map[string]string{"texte": " "}); code != http.StatusBadRequest {
		t.Fatalf("texte vide : %d", code)
	}
	if code, _ := lea.Post("/projets/etapes/"+etape+"/modifier", map[string]string{"texte": "X", "jour": "demain"}); code != http.StatusBadRequest {
		t.Fatalf("date invalide : %d", code)
	}
	// Plus de date ni de responsable.
	if _, page = marie.Post("/projets/etapes/"+etape+"/modifier", map[string]string{"texte": "Réserver le grand camion", "membre": "0"}); strings.Contains(page, `<option value="`+idLea+`" selected>`) {
		t.Fatal("responsable gardé")
	}

	// Projet privé de Léa : Marie ne modifie pas ses étapes.
	lea.Get("/projets/nouveau")
	_, page = lea.Post("/projets", map[string]string{"nom": "Cabane secrète", "prive": "1"})
	prive := essai.Trouver(t, page, `action="/projets/(\d+)/etapes"`)
	_, page = lea.Post("/projets/"+prive+"/etapes", map[string]string{"texte": "Trouver des planches"})
	etapePrivee := essai.Trouver(t, page, `action="/projets/etapes/(\d+)/modifier"`)
	marie.Get("/projets")
	if code, _ := marie.Post("/projets/etapes/"+etapePrivee+"/modifier", map[string]string{"texte": "Rien"}); code != http.StatusNotFound {
		t.Fatalf("étape d'un projet privé modifiée par un autre : %d", code)
	}
	if code, page := lea.Post("/projets/etapes/"+etapePrivee+"/modifier", map[string]string{"texte": "Trouver de belles planches"}); code != 200 || !strings.Contains(page, "Trouver de belles planches") {
		t.Fatalf("propriétaire : %d", code)
	}
}
