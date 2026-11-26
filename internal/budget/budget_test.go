// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
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
		t.Fatalf("disponible : %d\n%s", code, page)
	}
	if !strings.Contains(page, `class="jauge-mois"`) || !strings.Contains(page, "Supermarché") || !strings.Contains(page, "Dépenses du quotidien") {
		t.Fatal("jauge, opération ou dépenses du quotidien absentes")
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
	if _, page := marie.Get("/budget?mois=" + suivant); !strings.Contains(page, "à venir · le ") || !strings.Contains(page, "Salaire Marie") || !strings.Contains(page, "Prévu pour ce mois") {
		t.Fatal("salaire du mois prochain non prévu")
	}
	// L'année prochaine : 12 salaires prévus (et non une année vide).
	an := strconv.Itoa(auj.Year() + 1)
	if _, page := marie.Get("/budget/annee?an=" + an); !strings.Contains(page, "Reste prévu sur l") || !strings.Contains(page, appli.Montant(12*250000, "EUR")) ||
		!strings.Contains(page, `class="g-p0"`) {
		t.Fatalf("année prochaine sans le prévu :\n%s", page)
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
		// Dix mois : janvier à octobre, puis janvier suivant.
		{"2026-01-15", "mois10", 9, "2026-10-15"},
		{"2026-01-15", "mois10", 10, "2027-01-15"},
		{"2026-09-30", "mois10", 3, "2027-02-28"},
		{"2026-11-15", "mois10", 0, "2027-01-15"},
		// Année scolaire : septembre à juin, puis septembre suivant.
		{"2026-09-05", "scolaire", 3, "2026-12-05"},
		{"2026-09-05", "scolaire", 4, "2027-01-05"},
		{"2026-09-05", "scolaire", 9, "2027-06-05"},
		{"2026-09-05", "scolaire", 10, "2027-09-05"},
		{"2026-07-10", "scolaire", 0, "2026-09-10"},
		{"2026-01-31", "scolaire", 1, "2026-02-28"},
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
	if ParAn(10000, "scolaire") != 100000 || ParAn(10000, "mois10") != 100000 || ParMois(12000, "mois10") != 10000 || ParAn(100, "2mois") != 600 {
		t.Error("dix mois")
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

// Supprimer une récurrence avec ses opérations déjà ajoutées (réparation
// d'une ancienne opération « fixe » devenue récurrente).
func TestSupprimerAvecOccurrences(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	debut := time.Now().In(paris).AddDate(0, -3, 0)
	debut = time.Date(debut.Year(), debut.Month(), 1, 0, 0, 0, 0, paris)
	marie.Get("/budget/nouvelle")
	marie.Post("/budget", map[string]string{"montant": "50", "categorie": "loisirs", "jour": debut.Format("2006-01-02"), "frequence": "mois", "libelle": "Ancien club"})
	marie.Post("/budget", map[string]string{"montant": "10", "categorie": "courses", "jour": time.Now().In(paris).Format("2006-01-02"), "libelle": "Pain"})
	_, page := marie.Get("/budget/recurrences")
	id := essai.Trouver(t, page, `href="/recurrences/(\d+)"`)
	marie.Get("/recurrences/" + id)
	if _, page := marie.Post("/recurrences/"+id+"/supprimer", map[string]string{"passees": "1"}); !strings.Contains(page, "opérations ajoutées") {
		t.Fatalf("suppression :\n%s", page)
	}
	ops, _ := a.Q.OperationsPeriode(context.Background(), donnees.OperationsPeriodeParams{FamilleID: 1, Du: "2000-01-01", Au: "2999-01-01"})
	if len(ops) != 1 || ops[0].Libelle != "Pain" {
		t.Fatalf("%d opérations restantes", len(ops))
	}
}

// Nouveau montant prévu à l'avance : les fois d'avant gardent l'ancien
// montant, celles à partir de la date prennent le nouveau, qui devient le
// montant de la récurrence.
func TestNouveauMontantPrevu(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Now().In(paris)
	mois := func(n int) string {
		return time.Date(auj.Year(), auj.Month()+time.Month(n), 1, 0, 0, 0, 0, paris).Format("2006-01-02")
	}
	marie.Get("/budget/nouvelle")
	marie.Post("/budget", map[string]string{"montant": "700", "categorie": "impots", "jour": mois(0), "frequence": "mois", "libelle": "Taxe foncière"})
	_, page := marie.Get("/budget/recurrences")
	id := essai.Trouver(t, page, `href="/recurrences/(\d+)"`)
	if _, page = marie.Get("/recurrences/" + id); !strings.Contains(page, "Prévoir un nouveau montant") {
		t.Fatal("champ « nouveau montant » absent")
	}
	champs := map[string]string{"montant": "700", "categorie": "impots", "libelle": "Taxe foncière", "frequence": "mois",
		"prochaine": mois(1), "nouveau_montant": "750", "nouveau_le": auj.Format("2006-01-02")}
	if code, page := marie.Post("/recurrences/"+id, champs); code != http.StatusBadRequest || !strings.Contains(page, "après aujourd") {
		t.Fatalf("date du jour acceptée : %d", code)
	}
	champs["nouveau_le"] = ""
	if code, _ := marie.Post("/recurrences/"+id, champs); code != http.StatusBadRequest {
		t.Fatalf("montant sans date accepté : %d", code)
	}
	champs["nouveau_le"] = mois(2)
	marie.Post("/recurrences/"+id, champs)
	if _, page = marie.Get("/budget/recurrences"); !strings.Contains(page, "dès le") {
		t.Fatal("nouveau montant non affiché dans la liste")
	}
	ctx := context.Background()
	if err := Generer(ctx, a, 1, time.Date(auj.Year(), auj.Month()+3, 1, 12, 0, 0, 0, paris)); err != nil {
		t.Fatal(err)
	}
	ops, _ := a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: 1, Du: "2000-01-01", Au: "2999-01-01"})
	montants := map[string]int64{}
	for _, o := range ops {
		montants[o.Jour] = o.Montant
	}
	if montants[mois(0)] != 70000 || montants[mois(1)] != 70000 || montants[mois(2)] != 75000 || montants[mois(3)] != 75000 {
		t.Fatalf("montants : %v", montants)
	}
	rid, _ := strconv.ParseInt(id, 10, 64)
	r, _ := a.Q.RecurrenceParID(ctx, donnees.RecurrenceParIDParams{ID: rid, FamilleID: 1})
	if r.Montant != 75000 || r.NouveauLe != "" || r.NouveauMontant != 0 {
		t.Fatalf("récurrence après la date : %d, %q, %d", r.Montant, r.NouveauLe, r.NouveauMontant)
	}

	// Date du nouveau montant passée, mais la fois qui en profite pas encore
	// écrite (paiement en retard) : la fiche s'enregistre encore telle quelle ;
	// une autre date passée reste refusée.
	hier := auj.AddDate(0, 0, -1).Format("2006-01-02")
	if _, err := a.DB.ExecContext(ctx, `UPDATE recurrences SET nouveau_montant = 80000, nouveau_le = ? WHERE id = ?`, hier, rid); err != nil {
		t.Fatal(err)
	}
	_, page = marie.Get("/recurrences/" + id)
	champs = map[string]string{"montant": "750", "categorie": "impots", "libelle": "Taxe foncière (modifiée)", "frequence": "mois",
		"prochaine": essai.Trouver(t, page, `name="prochaine" value="([0-9-]+)"`), "nouveau_montant": "800", "nouveau_le": hier}
	if code, _ := marie.Post("/recurrences/"+id, champs); code != http.StatusOK {
		t.Fatalf("fiche avec un nouveau montant déjà daté refusée : %d", code)
	}
	champs["nouveau_le"] = auj.AddDate(0, 0, -2).Format("2006-01-02")
	if code, _ := marie.Post("/recurrences/"+id, champs); code != http.StatusBadRequest {
		t.Fatalf("autre date passée acceptée : %d", code)
	}
}

// Opération à date variable : rien n'est écrit tout seul ; elle attend
// « C'est passé » (vraie date, vrai montant) ou « Pas cette fois », et
// compte dans le reste à vivre en attendant.
func TestAConfirmer(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Now().In(paris).Format("2006-01-02")
	marie.Get("/budget/nouvelle")
	marie.Post("/budget", map[string]string{"montant": "45", "categorie": "ecole", "jour": auj, "frequence": "mois", "libelle": "Cantine", "a_confirmer": "1"})
	marie.Post("/budget", map[string]string{"montant": "30", "categorie": "remboursements", "jour": auj, "frequence": "mois", "libelle": "Mutuelle", "a_confirmer": "1"})
	ctx := context.Background()
	ops, _ := a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: 1, Du: "2000-01-01", Au: "2999-01-01"})
	if len(ops) != 0 {
		t.Fatalf("%d opérations écrites toutes seules", len(ops))
	}
	_, page := marie.Get("/budget?compte=commun")
	if !strings.Contains(page, "⏳ À confirmer") || !strings.Contains(page, "À prélever 45,00") || !strings.Contains(page, "Revenus du mois : <b>30,00") {
		t.Fatal("à confirmer : absent de la page ou pas compté dans le mois")
	}
	cantine := essai.Trouver(t, page, `Cantine<small>[^<]*</small></span>[\s\S]*?action="/recurrences/(\d+)/confirmer"`)
	// Mauvais numéro de fois : rien n'est fait.
	if code, _ := marie.Post("/recurrences/"+cantine+"/confirmer", map[string]string{"n": "3", "montant": "47", "jour": auj}); code != http.StatusConflict {
		t.Fatalf("mauvais numéro accepté : %d", code)
	}
	if code, p := marie.Post("/recurrences/"+cantine+"/confirmer", map[string]string{"n": "0", "montant": "47,50", "jour": auj}); code != http.StatusOK || !strings.Contains(p, "Ajouté au budget") {
		t.Fatalf("confirmer : %d", code)
	}
	// Deux fois le même envoi : une seule opération.
	marie.Post("/recurrences/"+cantine+"/confirmer", map[string]string{"n": "0", "montant": "47,50", "jour": auj})
	_, page = marie.Get("/budget?compte=commun")
	mutuelle := essai.Trouver(t, page, `Mutuelle<small>[^<]*</small></span>[\s\S]*?action="/recurrences/(\d+)/confirmer"`)
	if mutuelle == cantine {
		t.Fatal("la cantine attend encore")
	}
	if code, p := marie.Post("/recurrences/"+mutuelle+"/passer", map[string]string{"n": "0"}); code != http.StatusOK || !strings.Contains(p, "Pas cette fois") {
		t.Fatalf("passer : %d", code)
	}
	ops, _ = a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: 1, Du: "2000-01-01", Au: "2999-01-01"})
	if len(ops) != 1 || ops[0].Montant != 4750 || ops[0].Libelle != "Cantine" {
		t.Fatalf("opérations : %+v", ops)
	}
	if _, page = marie.Get("/budget?compte=commun"); strings.Contains(page, "⏳ À confirmer") {
		t.Fatal("encore des opérations à confirmer")
	}
	if _, page = marie.Get("/budget/recurrences?compte=commun"); !strings.Contains(page, "⏳ à confirmer ·") {
		t.Fatal("étiquette « à confirmer » absente de la liste")
	}
	// Date variable : la date peut rester vide (ce mois-ci, dès le 1er) ;
	// sans « Date variable », elle est demandée.
	if code, _ := marie.Post("/budget", map[string]string{"montant": "20", "categorie": "beaute", "jour": "", "frequence": "mois", "libelle": "Coiffeur", "a_confirmer": "1"}); code != 200 {
		t.Fatalf("date variable sans date : %d", code)
	}
	if code, p := marie.Post("/budget", map[string]string{"montant": "20", "categorie": "beaute", "jour": "", "libelle": "Sans date"}); code != 400 || !strings.Contains(p, "Date variable") {
		t.Fatalf("opération sans date acceptée : %d", code)
	}
	recs, _ := a.Q.RecurrencesFamille(ctx, 1)
	premier := time.Now().In(paris).Format("2006-01") + "-01"
	trouve := false
	for _, r := range recs {
		if r.Libelle == "Coiffeur" && r.Debut == premier && r.AConfirmer == 1 {
			trouve = true
		}
	}
	if !trouve {
		t.Fatal("date variable sans date : pas au 1er du mois")
	}
	if _, page = marie.Get("/budget?compte=commun"); !strings.Contains(page, "attendu en ") {
		t.Fatal("« attendu en <mois> » absent")
	}
}

// Les prochains mois : 12 mois d'après les opérations qui reviennent ; ce
// qui change d'un mois à l'autre (« + » une fois par an, « pas de » l'été).
func TestProchainsMois(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	maint := essai.Maintenant()
	premier := func(n int) string {
		return time.Date(maint.Year(), maint.Month()+time.Month(n), 1, 0, 0, 0, 0, maint.Location()).Format("2006-01-02")
	}
	marie.Get("/budget/nouvelle")
	for _, f := range []map[string]string{
		{"montant": "2000", "categorie": "salaire", "jour": premier(1), "frequence": "mois", "libelle": "Salaire"},
		{"montant": "700", "categorie": "logement", "jour": premier(1), "frequence": "mois", "libelle": "Loyer"},
		{"montant": "300", "categorie": "assurances", "jour": premier(3), "frequence": "annee", "libelle": "Assurance"},
	} {
		if code, _ := marie.Post("/budget", f); code != 200 {
			t.Fatalf("création %v : %d", f, code)
		}
	}
	code, page := marie.Get("/budget/prochains?compte=commun")
	if code != 200 || strings.Count(page, `class="mois-prevu`) != 12 {
		t.Fatalf("prochains mois : %d, %d mois", code, strings.Count(page, `class="mois-prevu`))
	}
	if !strings.Contains(page, appli.Montant(130000, "EUR")) || !strings.Contains(page, appli.Montant(100000, "EUR")) || !strings.Contains(page, "&#43; Assurance") {
		t.Fatal("reste des mois (1 300 / 1 000 avec l'assurance) ou « + Assurance » absents")
	}
	if !strings.Contains(page, `href="/budget?compte=commun&amp;mois=`+premier(3)[:7]+`"`) {
		t.Fatal("lien vers la page du mois")
	}
}
