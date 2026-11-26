// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"familycocon/internal/essai"
)

// Outils du budget : formulaire selon le sens, plafonds et alerte, payé par,
// recherche sans accents, vue de l'année.
func TestOutilsBudget(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	essai.Enfant(t, srv, marie, "Léa")
	var idMarie, idLea int64
	if err := a.DB.QueryRow(`SELECT id FROM membres WHERE nom = 'Léa'`).Scan(&idLea); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.QueryRow(`SELECT id FROM membres WHERE nom = 'Marie'`).Scan(&idMarie); err != nil {
		t.Fatal(err)
	}
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Now().In(paris).Format("2006-01-02")

	// « Nouveau revenu » : pas de catégorie de dépense.
	_, form := marie.Get("/budget/nouvelle?sens=revenu")
	if strings.Contains(form, `value="courses"`) || !strings.Contains(form, `value="salaire"`) || !strings.Contains(form, "est une dépense ?") {
		t.Fatalf("formulaire de revenu avec les catégories de dépense :\n%s", form)
	}
	_, form = marie.Get("/budget/nouvelle")
	if strings.Contains(form, `value="salaire"`) || !strings.Contains(form, `value="impots"`) {
		t.Fatal("formulaire de dépense : catégories")
	}

	marie.Post("/budget", map[string]string{"montant": "95", "categorie": "courses", "libelle": "Supermarché", "jour": auj, "membre": strconv.FormatInt(idMarie, 10)})
	marie.Post("/budget", map[string]string{"montant": "40", "categorie": "sante", "libelle": "Dentiste Hélène", "jour": auj, "membre": strconv.FormatInt(idLea, 10)})

	// Libellés proposés.
	if _, form = marie.Get("/budget/nouvelle"); !strings.Contains(form, `<option value="Supermarché" data-cat="courses">`) ||
		!strings.Contains(form, `<option value="Dentiste Hélène" data-cat="sante">`) || !strings.Contains(form, "data-cat-auto") {
		t.Fatalf("libellés déjà tapés non proposés avec leur catégorie :\n%s", form)
	}
	// Payé par : deux personnes, écart de 55 €.
	_, page := marie.Get("/budget")
	if !strings.Contains(page, "Payé par") || !strings.Contains(page, "55,00") {
		t.Fatalf("bloc « Payé par » :\n%s", page)
	}
	// Recherche sans accents.
	if _, page = marie.Get("/budget/chercher?q=" + url.QueryEscape("HELENE")); !strings.Contains(page, "Dentiste Hélène") || strings.Contains(page, "Supermarché") || !strings.Contains(page, "40,00") {
		t.Fatalf("recherche :\n%s", page)
	}
	// Chercher par catégorie seule, sur tout l'historique.
	if _, page = marie.Get("/budget/chercher?cat=sante&periode=tout"); !strings.Contains(page, "Dentiste Hélène") || strings.Contains(page, "Supermarché") ||
		!strings.Contains(page, "1 opération trouvée") {
		t.Fatalf("recherche par catégorie :\n%s", page)
	}
	// Texte et catégorie qui ne vont pas ensemble : rien.
	if _, page = marie.Get("/budget/chercher?q=super&cat=sante"); !strings.Contains(page, "Rien trouvé pour") {
		t.Fatalf("recherche texte + catégorie :\n%s", page)
	}
	// Année : le mois en cours a 135 € de dépenses.
	if _, page = marie.Get("/budget/annee"); !strings.Contains(page, "135,00") || !strings.Contains(page, "Mois par mois") || !strings.Contains(page, `class="graphique"`) || strings.Contains(page, "<progress") {
		t.Fatalf("vue de l'année :\n%s", page)
	}
}
