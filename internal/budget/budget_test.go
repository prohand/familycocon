// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
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
	marie.Post("/budget", map[string]string{"montant": "2 500", "categorie": "salaire", "jour": jour, "frequence": "mois", "libelle": "Salaire Marie"})
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
	// Mois suivant : le salaire est prévu, puis écrit tout seul le jour venu.
	suivant := time.Date(auj.Year(), auj.Month()+1, 1, 0, 0, 0, 0, paris).Format("2006-01")
	if _, page := marie.Get("/budget?mois=" + suivant); !strings.Contains(page, "À venir ce mois-ci") || !strings.Contains(page, "Salaire Marie") {
		t.Fatal("salaire du mois prochain non prévu")
	}
	_, page = marie.Get("/budget/recurrences")
	if !strings.Contains(page, "Salaire Marie") || !strings.Contains(page, "Chaque mois") {
		t.Fatalf("page des récurrences :\n%s", page)
	}
	id := essai.Trouver(t, page, `href="/recurrences/(\d+)"`)
	if code, _ := marie.Post("/recurrences/"+id+"/arreter", nil); code != 200 {
		t.Fatalf("arrêter : %d", code)
	}
	if _, page := marie.Get("/budget?mois=" + suivant); strings.Contains(page, "Salaire Marie") {
		t.Fatal("récurrence arrêtée encore prévue")
	}
	// Réservé aux parents.
	if code, _ := lea.Get("/budget"); code != http.StatusForbidden {
		t.Fatalf("budget ouvert à un enfant : %d", code)
	}
}

func TestOccurrence(t *testing.T) {
	cas := []struct {
		debut, freq string
		k           int64
		attendu     string
	}{
		{"2026-01-31", "mois", 1, "2026-02-28"},
		{"2026-01-31", "mois", 2, "2026-03-31"},
		{"2024-02-29", "annee", 1, "2025-02-28"},
		{"2026-11-30", "trimestre", 1, "2027-02-28"},
		{"2026-11-22", "semaine", 2, "2026-12-06"},
	}
	for _, c := range cas {
		j, err := Occurrence(c.debut, c.freq, c.k)
		if err != nil || j.Format("2006-01-02") != c.attendu {
			t.Errorf("%s %s n°%d : %v, attendu %s", c.debut, c.freq, c.k, j, c.attendu)
		}
	}
	if j, _ := Occurrence("2026-08-31", "semestre", 1); j.Format("2006-01-02") != "2027-02-28" {
		t.Errorf("semestre : %v", j)
	}
	if ParMois(1200, "annee") != 100 || ParMois(1200, "semaine") != 5200 || ParMois(1200, "semestre") != 200 {
		t.Error("ParMois")
	}
}

// Une récurrence commencée il y a 3 mois : 4 occurrences écrites, une seule
// fois même si on génère deux fois.
func TestGenerer(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Now().In(paris)
	debut := auj.AddDate(0, -3, 0)
	if debut.Day() != auj.Day() { // fin de mois : on évite le cas limite ici
		debut = time.Date(auj.Year(), auj.Month()-3, 1, 0, 0, 0, 0, paris)
	}
	marie.Get("/budget/nouvelle")
	marie.Post("/budget", map[string]string{"montant": "700", "categorie": "logement", "jour": debut.Format("2006-01-02"), "frequence": "mois", "libelle": "Loyer"})
	ctx := context.Background()
	f, err := a.Q.ToutesFamilles(ctx)
	if err != nil || len(f) != 1 {
		t.Fatal(err)
	}
	for range 2 {
		if err := Generer(ctx, a, f[0].ID, auj); err != nil {
			t.Fatal(err)
		}
	}
	ops, _ := a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: f[0].ID, Du: "2000-01-01", Au: "2999-01-01"})
	if len(ops) != 4 {
		t.Fatalf("%d occurrences, attendu 4", len(ops))
	}
}
