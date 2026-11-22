// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/essai"
)

func TestLireMontant(t *testing.T) {
	bons := map[string]int64{"12": 1200, "12,5": 1250, "12.50": 1250, "1 234,56 €": 123456, ",5": 50, "0,01": 1, "1 000": 100000}
	for s, attendu := range bons {
		if v, err := LireMontant(s); err != nil || v != attendu {
			t.Errorf("%q : %d (%v), attendu %d", s, v, err, attendu)
		}
	}
	for _, s := range []string{"", "0", "-5", "12,345", "abc", "1.2.3", "99999999999"} {
		if _, err := LireMontant(s); err == nil {
			t.Errorf("%q accepté", s)
		}
	}
}

func TestCourbeSVG(t *testing.T) {
	svg := string(Courbe{Actuel: []int64{0, 1000, 5000}, Precedent: []int64{100, 200, 300, 900}, Jours: 30, Devise: "EUR"}.SVG())
	for _, attendu := range []string{`<svg class="courbe"`, `class="actuel"`, `class="precedent"`, `class="point"`, "50 €"} {
		if !strings.Contains(svg, attendu) {
			t.Errorf("%q absent du SVG", attendu)
		}
	}
	if strings.Contains(svg, "style=") {
		t.Error("style dans le SVG (interdit par la CSP)")
	}
	// Mois vide : pas de plantage.
	_ = Courbe{Jours: 31}.SVG()
	if arrondiHaut(123400) != 200000 || arrondiHaut(4500) != 5000 {
		t.Error("arrondi de l'échelle")
	}
}

func TestBudget(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Now().In(paris)
	jour := auj.Format("2006-01-02")

	marie.Get("/budget/nouvelle")
	marie.Post("/budget", map[string]string{"montant": "2 500", "categorie": "salaire", "jour": jour, "fixe": "1", "libelle": "Salaire Marie"})
	marie.Post("/budget", map[string]string{"montant": "84,30", "categorie": "courses", "jour": jour, "libelle": "Supermarché"})
	code, page := marie.Post("/budget", map[string]string{"montant": "12,5", "categorie": "loisirs", "jour": jour})
	if code != 200 || !strings.Contains(page, "2 403,20 €") {
		t.Fatalf("reste à vivre : %d\n%s", code, page)
	}
	if !strings.Contains(page, `<svg class="courbe"`) || !strings.Contains(page, "Supermarché") {
		t.Fatal("courbe ou opération absente")
	}
	if code, _ := marie.Post("/budget", map[string]string{"montant": "-3", "categorie": "courses", "jour": jour}); code != 400 {
		t.Fatalf("montant négatif accepté : %d", code)
	}
	if code, _ := marie.Post("/budget", map[string]string{"montant": "3", "categorie": "casino", "jour": jour}); code != 400 {
		t.Fatalf("catégorie inconnue acceptée : %d", code)
	}
	// Filtre par catégorie.
	if _, page := marie.Get("/budget?cat=loisirs"); strings.Contains(page, "Supermarché") || !strings.Contains(page, "Loisirs") {
		t.Fatal("filtre par catégorie")
	}
	// Mois suivant : le salaire « fixe » se recopie.
	suivant := time.Date(auj.Year(), auj.Month()+1, 1, 0, 0, 0, 0, paris).Format("2006-01")
	if _, page := marie.Get("/budget?mois=" + suivant); !strings.Contains(page, "Recopier 1 opération fixe") {
		t.Fatal("proposition de recopie absente")
	}
	if code, page := marie.Post("/budget/fixes", map[string]string{"mois": suivant}); code != 200 || !strings.Contains(page, "Salaire Marie") {
		t.Fatalf("recopie : %d", code)
	}
	if _, page := marie.Get("/budget?mois=" + suivant); strings.Contains(page, "Recopier") {
		t.Fatal("recopie proposée deux fois")
	}
	// Réservé aux parents.
	if code, _ := lea.Get("/budget"); code != http.StatusForbidden {
		t.Fatalf("budget ouvert à un enfant : %d", code)
	}
}
