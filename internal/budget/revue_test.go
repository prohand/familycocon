// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
	"familycocon/internal/repetition"
)

// Revue du 23/11/2026 : montants, dates, courbe.
func TestRevueSaisies(t *testing.T) {
	if v, err := LireMontantOuZero("0"); err != nil || v != 0 {
		t.Errorf("« 0 » refusé : %d, %v", v, err)
	}
	if _, err := LireMontant("0"); err == nil {
		t.Error("LireMontant accepte 0")
	}
	if v, err := LireMontantOuZero("12,5"); err != nil || v != 1250 {
		t.Errorf("12,5 : %d, %v", v, err)
	}
	auj := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	for jour, attendu := range map[string]bool{"2026-11-23": true, "1900-01-01": true, "2076-12-31": true,
		"9999-06-01": false, "1899-12-31": false, "2077-01-01": false, "": false, "2026-02-30": false} {
		if _, ok := LireJour(jour, auj); ok != attendu {
			t.Errorf("LireJour(%q) = %v", jour, ok)
		}
	}
	if s := court(150000, "CAD"); !strings.Contains(s, "$ CA") {
		t.Errorf("symbole du dollar canadien : %q", s)
	}
}

// Le mois précédent est tracé à la même échelle que le mois affiché : son
// jour n est sous le jour n, et ses jours en trop ne dépassent pas.
func TestRevueCourbeMoisPrecedent(t *testing.T) {
	prec := make([]int64, 31)
	act := make([]int64, 28)
	for i := range prec {
		prec[i] = int64(i+1) * 100
	}
	for i := range act {
		act[i] = int64(i+1) * 100
	}
	svg := string(Courbe{Actuel: act, Precedent: prec, Jours: 28, Devise: "EUR"}.SVG())
	points := func(classe string) []string {
		m := regexp.MustCompile(`<polyline class="` + classe + `" points="([^"]+)"`).FindStringSubmatch(svg)
		if m == nil {
			t.Fatalf("courbe %q absente", classe)
		}
		return strings.Fields(m[1])
	}
	p, a := points("precedent"), points("actuel")
	if len(p) != 28 {
		t.Fatalf("%d points pour le mois précédent, attendu 28", len(p))
	}
	for _, i := range []int{0, 14, 27} {
		if strings.Split(p[i], ",")[0] != strings.Split(a[i], ",")[0] {
			t.Errorf("jour %d : abscisses différentes (%s et %s)", i+1, p[i], a[i])
		}
	}
}

func TestRevueBudget(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	ctx := context.Background()
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Now().In(paris)
	jour := repetition.CleJour(auj)

	// Double envoi : une seule opération.
	marie.Get("/budget/nouvelle")
	op := map[string]string{"montant": "42", "categorie": "courses", "libelle": "Marché", "jour": jour}
	marie.Post("/budget", op)
	marie.Post("/budget", op)
	ops, _ := a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: 1, Du: jour, Au: jour})
	if len(ops) != 1 {
		t.Fatalf("double envoi : %d opérations", len(ops))
	}
	// Une autre opération (montant différent) passe bien.
	op["montant"] = "43"
	marie.Post("/budget", op)
	if ops, _ := a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: 1, Du: jour, Au: jour}); len(ops) != 2 {
		t.Fatalf("seconde opération : %d", len(ops))
	}
	// Date absurde refusée.
	op["jour"] = "9999-06-01"
	if code, _ := marie.Post("/budget", op); code != http.StatusBadRequest {
		t.Fatalf("date en 9999 : %d", code)
	}

	// Récurrence : modifiée seulement si « faites » n'a pas bougé depuis la lecture.
	rec, err := a.Q.CreerRecurrence(ctx, donnees.CreerRecurrenceParams{FamilleID: 1, Sens: "depense", Montant: 1000, Categorie: "logement",
		Libelle: "Loyer", Frequence: "mois", Debut: repetition.CleJour(auj.AddDate(0, 0, 3)), CreeLe: time.Now().Unix()})
	if err != nil {
		t.Fatal(err)
	}
	modif := donnees.ModifierRecurrenceParams{Sens: "depense", Montant: 1100, Categorie: "logement", Libelle: "Loyer", Frequence: "mois",
		Debut: rec.Debut, Faites: 0, ID: rec.ID, FamilleID: 1, FaitesAvant: 5}
	if n, _ := a.Q.ModifierRecurrence(ctx, modif); n != 0 {
		t.Fatal("récurrence réécrite malgré un compteur différent")
	}
	modif.FaitesAvant = 0
	if n, _ := a.Q.ModifierRecurrence(ctx, modif); n != 1 {
		t.Fatal("récurrence non modifiée")
	}
	// Une autre famille ne peut pas avancer la récurrence.
	if n, _ := a.Q.AvancerRecurrence(ctx, donnees.AvancerRecurrenceParams{ID: rec.ID, FamilleID: 2, Faites: 0}); n != 0 {
		t.Fatal("récurrence avancée par une autre famille")
	}

	// GenererSiBesoin : une fois par jour ; Generer écrit toujours.
	if err := GenererSiBesoin(ctx, a, 1, auj); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Q.CreerRecurrence(ctx, donnees.CreerRecurrenceParams{FamilleID: 1, Sens: "depense", Montant: 500, Categorie: "energie",
		Libelle: "Gaz", Frequence: "mois", Debut: jour, CreeLe: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	compter := func() int {
		ops, _ := a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: 1, Du: jour, Au: jour})
		return len(ops)
	}
	avant := compter()
	_ = GenererSiBesoin(ctx, a, 1, auj)
	if compter() != avant {
		t.Fatal("GenererSiBesoin a tout relu alors que c'était fait aujourd'hui")
	}
	_ = Generer(ctx, a, 1, auj)
	if compter() != avant+1 {
		t.Fatal("Generer n'a pas écrit l'occurrence du jour")
	}
	// Le lendemain, GenererSiBesoin retravaille.
	if j, _ := dejaGenere.Load(cleGenere{a, 1}); j != jour {
		t.Fatalf("mémo : %v", j)
	}

	// Replanifier : rien ne bouge si la date et la fréquence sont les mêmes.
	x, _ := a.Q.RecurrenceParID(ctx, donnees.RecurrenceParIDParams{ID: rec.ID, FamilleID: 1})
	p, _ := Prochaine(x)
	if d, f, msg := Replanifier(x, repetition.CleJour(p), x.Frequence, auj); d != x.Debut || f != x.Faites || msg != "" {
		t.Fatalf("Replanifier sans changement : %q %d %q", d, f, msg)
	}
	nouvelle := repetition.CleJour(auj.AddDate(0, 0, 10))
	if d, f, msg := Replanifier(x, nouvelle, x.Frequence, auj); d != nouvelle || f != 0 || msg != "" {
		t.Fatalf("Replanifier avec une nouvelle date : %q %d %q", d, f, msg)
	}
	// Plafond : message à part de l'erreur SQL.
	if msg, err := PlaceRecurrence(ctx, a, 1); msg != "" || err != nil {
		t.Fatalf("PlaceRecurrence : %q %v", msg, err)
	}
}
