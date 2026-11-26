// SPDX-License-Identifier: GPL-3.0-or-later

package menus

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"familycocon/internal/essai"
)

// Semaine en cours : les jours déjà passés sont repliés ; une semaine
// passée reste entière.
func TestJoursPasses(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	maintenant := essai.Maintenant()
	_, page := marie.Get("/menus")
	if maintenant.Weekday() == time.Monday {
		if strings.Contains(page, `class="jours-passes"`) {
			t.Fatal("lundi : aucun jour passé à replier")
		}
	} else {
		hier := maintenant.AddDate(0, 0, -1)
		attendu := "passé"
		if maintenant.Weekday() != time.Tuesday {
			attendu = "passés"
		}
		if !strings.Contains(page, `class="jours-passes"`) || !strings.Contains(page, "Lundi") || !strings.Contains(page, attendu+" ▸") ||
			strings.Count(page, "jour-menu passe") != (int(maintenant.Weekday())+6)%7 {
			t.Fatalf("jours passés (hier %s) :\n%s", hier.Format("2006-01-02"), page)
		}
		if !strings.Contains(page, "jour-menu aujourdhui\"") {
			t.Fatal("le jour courant ne doit pas être replié")
		}
	}
	_, page = marie.Get("/menus?date=" + maintenant.AddDate(0, 0, -14).Format("2006-01-02"))
	if strings.Contains(page, `class="jours-passes"`) || strings.Contains(page, "jour-menu passe") {
		t.Fatal("semaine passée repliée")
	}
}

// « ＋ Recette » sans repas, préparation mise en forme, « Prévoir ce plat ».
func TestNouvelleRecetteEtPrevoir(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	if _, page := marie.Get("/menus/recettes"); !strings.Contains(page, `href="/menus/recettes/nouvelle"`) {
		t.Fatal("bouton « ＋ Recette » absent")
	}
	if _, page := marie.Get("/menus/recettes/nouvelle"); !strings.Contains(page, `name="preparation"`) || !strings.Contains(page, "data-riche") {
		t.Fatal("formulaire de recette sans préparation")
	}
	code, page := marie.Post("/menus/recettes", map[string]string{"nom": "Gratin dauphinois",
		"ingredients": "1 kg de pommes de terre\n50 cl de crème", "preparation": "Cuire **1 heure** à 180°."})
	if code != 200 || !strings.Contains(page, "Gratin dauphinois") || !strings.Contains(page, "<strong>1 heure</strong>") ||
		!strings.Contains(page, "jamais prévu") || !strings.Contains(page, "<li>50 cl de crème</li>") {
		t.Fatalf("recette créée : %d\n%s", code, page)
	}
	// Même nom : refusé, rien d'écrasé.
	if code, _ := marie.Post("/menus/recettes", map[string]string{"nom": "gratin DAUPHINOIS"}); code != http.StatusConflict {
		t.Fatalf("doublon accepté : %d", code)
	}
	if code, _ := marie.Post("/menus/recettes", map[string]string{"nom": "Long", "preparation": strings.Repeat("a", 5001)}); code != http.StatusBadRequest {
		t.Fatalf("préparation trop longue acceptée : %d", code)
	}
	cle := essai.Trouver(t, page, `name="cle" value="([^"]+)"`)
	// Prévoir ce plat : le repas est rempli, la recette comptée.
	code, page = marie.Post("/menus/recettes/prevoir", map[string]string{"cle": cle, "jour": "2026-12-02", "moment": "soir"})
	if code != 200 || !strings.Contains(page, "Gratin dauphinois") || !strings.Contains(page, "2 ingrédients") {
		t.Fatalf("prévoir : %d", code)
	}
	if _, page := marie.Get("/menus/recettes"); !strings.Contains(page, "prévu 1 fois") {
		t.Fatal("recette prévue non comptée")
	}
	// Repas déjà prévu : pas écrasé.
	marie.Post("/menus/recettes", map[string]string{"nom": "Soupe"})
	_, page = marie.Get("/menus/recettes")
	soupe := essai.Trouver(t, page, `name="cle" value="(soupe)"`)
	if code, _ := marie.Post("/menus/recettes/prevoir", map[string]string{"cle": soupe, "jour": "2026-12-02", "moment": "soir"}); code != http.StatusConflict {
		t.Fatalf("repas écrasé : %d", code)
	}
	if code, _ := marie.Post("/menus/recettes/prevoir", map[string]string{"cle": soupe, "jour": "2026-12-02", "moment": "gouter"}); code != http.StatusBadRequest {
		t.Fatalf("repas inconnu accepté : %d", code)
	}
	// La préparation se lit aussi sur la page du repas.
	if _, page := marie.Get("/menus/2026-12-02/soir"); !strings.Contains(page, "<strong>1 heure</strong>") {
		t.Fatal("préparation absente de la page du repas")
	}
	// Modifier : la préparation suit, même renommée.
	if _, page := marie.Get("/menus/recettes/modifier?cle=" + url.QueryEscape(cle)); !strings.Contains(page, "Cuire **1 heure**") {
		t.Fatalf("formulaire de modification :\n%s", page)
	}
	marie.Post("/menus/recettes/modifier", map[string]string{"cle": cle, "nom": "Gratin du dimanche",
		"ingredients": "Pommes de terre", "preparation": "_Au four_"})
	if _, page := marie.Get("/menus/recettes"); !strings.Contains(page, "Gratin du dimanche") || !strings.Contains(page, "<em>Au four</em>") {
		t.Fatal("préparation non modifiée")
	}
	if code, _ := marie.Get("/menus/recettes/modifier?cle=inconnue"); code != http.StatusNotFound {
		t.Fatalf("recette inconnue : %d", code)
	}
	// Un enfant crée et prévoit aussi (tout le monde propose un plat).
	lea := essai.Enfant(t, srv, marie, "Léa")
	lea.Get("/menus/recettes")
	if code, _ := lea.Post("/menus/recettes", map[string]string{"nom": "Crêpes"}); code != 200 {
		t.Fatalf("recette d'un enfant : %d", code)
	}
}
