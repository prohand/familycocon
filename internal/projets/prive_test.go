// SPDX-License-Identifier: GPL-3.0-or-later

package projets

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"familycocon/internal/abonnements"
	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/donnees"
	"familycocon/internal/essai"
	"familycocon/internal/suivi"
)

// Projet et abonnement privés : vus par leur propriétaire seul ; dépense
// du carnet envoyée au budget perso ; tout part avec le membre retiré.
func TestPriveProjetsAbonnements(t *testing.T) {
	a, srv := essai.NouvelleApp(t, func(a *appli.App) {
		budget.Brancher(a)
		abonnements.Brancher(a)
		Brancher(a, suivi.Brancher(a))
	})
	marie := essai.Famille(t, srv)
	paul := essai.Parent(t, srv, marie, "Paul")
	lea := essai.Enfant(t, srv, marie, "Léa")
	auj := essai.Maintenant().Format("2006-01-02")
	mois := auj[:7]

	// Paul crée un projet privé, avec une étape datée.
	paul.Get("/projets/nouveau")
	_, page := paul.Post("/projets", map[string]string{"nom": "Cadeau surprise", "prive": "1", "objectif": essai.Maintenant().AddDate(0, 0, 5).Format("2006-01-02")})
	id := essai.Trouver(t, page, `action="/projets/(\d+)/etapes"`)
	if !strings.Contains(page, "Projet privé") || !strings.Contains(page, `<option value="perso" selected>`) {
		t.Fatal("projet privé : mention ou budget perso proposé d'abord")
	}
	paul.Post("/projets/"+id+"/etapes", map[string]string{"texte": "Commander le cadeau", "jour": auj})
	// Dépense du carnet, envoyée au budget perso de Paul.
	paul.Post("/suivi/projets/"+id+"/carnet", map[string]string{"libelle": "Acompte bijouterie", "jour": auj, "montant": "150", "budget": "perso"})
	if _, b := paul.Get("/budget?compte=perso&mois=" + mois); !strings.Contains(b, "Acompte bijouterie") {
		t.Fatal("dépense absente du budget perso de Paul")
	}
	if _, b := marie.Get("/budget?compte=commun&mois=" + mois); strings.Contains(b, "Acompte bijouterie") {
		t.Fatal("dépense privée dans le compte commun")
	}
	// Marie et Léa ne voient rien : liste, fiche, recherche, étapes, carnet, rappels.
	for nom, n := range map[string]*essai.Navigateur{"Marie": marie, "Léa": lea} {
		if _, p := n.Get("/projets"); strings.Contains(p, "Cadeau surprise") {
			t.Errorf("%s voit le projet dans la liste", nom)
		}
		if code, _ := n.Get("/projets/" + id); code != http.StatusNotFound {
			t.Errorf("%s ouvre le projet : %d", nom, code)
		}
		if _, p := n.Get("/chercher?q=surprise"); strings.Contains(p, "Cadeau surprise") {
			t.Errorf("%s trouve le projet", nom)
		}
		if _, p := n.Get("/"); strings.Contains(p, "Commander le cadeau") || strings.Contains(p, "Cadeau surprise") {
			t.Errorf("%s voit les rappels du projet", nom)
		}
		if code, _ := n.Post("/suivi/projets/"+id+"/carnet", map[string]string{"libelle": "X", "jour": auj}); code != http.StatusNotFound {
			t.Errorf("%s écrit dans le carnet : %d", nom, code)
		}
	}
	if _, p := paul.Get("/"); !strings.Contains(p, "Commander le cadeau") {
		t.Error("Paul ne voit pas son rappel")
	}
	// Le projet redevient partagé : Marie le voit.
	paul.Post("/projets/"+id, map[string]string{"nom": "Cadeau surprise"})
	if _, p := marie.Get("/projets"); !strings.Contains(p, "Cadeau surprise") {
		t.Fatal("projet partagé invisible")
	}
	// Un projet partagé ne redevient pas privé.
	paul.Post("/projets/"+id, map[string]string{"nom": "Cadeau surprise", "prive": "1"})
	if _, p := marie.Get("/projets"); !strings.Contains(p, "Cadeau surprise") {
		t.Fatal("projet partagé rendu privé")
	}

	// Abonnement privé de Paul : dans son budget perso, invisible pour Marie.
	paul.Get("/abonnements/nouveau")
	paul.Post("/abonnements", map[string]string{"nom": "Magazine de Paul", "montant": "9,90", "frequence": "mois", "prochain": auj, "prive": "1"})
	if _, p := marie.Get("/abonnements"); strings.Contains(p, "Magazine de Paul") {
		t.Fatal("Marie voit l'abonnement privé de Paul")
	}
	_, p := paul.Get("/abonnements")
	if !strings.Contains(p, "🔒 Magazine de Paul") {
		t.Fatal("abonnement privé absent pour Paul")
	}
	ab := essai.Trouver(t, p, `href="/abonnements/(\d+)"`)
	if code, _ := marie.Get("/abonnements/" + ab); code != http.StatusNotFound {
		t.Fatalf("Marie ouvre l'abonnement privé : %d", code)
	}
	if _, b := paul.Get("/budget?compte=perso&mois=" + mois); !strings.Contains(b, "Magazine de Paul") {
		t.Fatal("prélèvement absent du budget perso")
	}
	if _, b := marie.Get("/budget?compte=commun&mois=" + mois); strings.Contains(b, "Magazine de Paul") {
		t.Fatal("prélèvement privé au compte commun")
	}
	// Rendu partagé : le prélèvement déjà écrit passe au commun.
	paul.Post("/abonnements/"+ab, map[string]string{"nom": "Magazine de Paul", "montant": "9,90", "frequence": "mois", "prochain": essai.Maintenant().AddDate(0, 1, 0).Format("2006-01-02")})
	if _, b := marie.Get("/budget?compte=commun&mois=" + mois); !strings.Contains(b, "Magazine de Paul") {
		t.Fatal("prélèvement non passé au commun")
	}

	// Paul a un nouveau projet privé (avec une photo au carnet) et un budget perso ;
	// Marie le retire de la famille : tout ce qui était privé à Paul part.
	_, page = paul.Post("/projets", map[string]string{"nom": "Projet secret", "prive": "1"})
	id2 := essai.Trouver(t, page, `action="/projets/(\d+)/etapes"`)
	paul.Post("/suivi/projets/"+id2+"/carnet", map[string]string{"libelle": "Achat", "jour": auj, "montant": "10"})
	marie.Get("/famille")
	_, fam := marie.Get("/famille")
	idPaul := essai.Trouver(t, fam, `action="/famille/membres/(\d+)/retirer"`)
	if code, _ := marie.Post("/famille/membres/"+idPaul+"/retirer", nil); code != 200 {
		t.Fatalf("retrait de Paul : %d", code)
	}
	var reste int
	if err := a.DB.QueryRow("SELECT count(*) FROM projets WHERE perso_de = ?", idPaul).Scan(&reste); err != nil || reste != 0 {
		t.Fatalf("projets privés de Paul restés : %d %v", reste, err)
	}
	if err := a.DB.QueryRow("SELECT count(*) FROM operations WHERE perso_de = ?", idPaul).Scan(&reste); err != nil || reste != 0 {
		t.Fatalf("budget perso de Paul resté : %d %v", reste, err)
	}
	if err := a.DB.QueryRow("SELECT count(*) FROM carnet WHERE module = 'projets' AND objet_id = ?", id2).Scan(&reste); err != nil || reste != 0 {
		t.Fatalf("carnet du projet privé resté : %d %v", reste, err)
	}
}

// Ligne du carnet d'un projet partagé envoyée au budget perso de Marie :
// Paul la corrige ou la supprime sans toucher au budget perso de Marie ;
// Marie vide le coût : la dépense quitte son budget.
func TestCarnetBudgetPersoDUnAutre(t *testing.T) {
	a, srv := essai.NouvelleApp(t, func(a *appli.App) {
		budget.Brancher(a)
		Brancher(a, suivi.Brancher(a))
	})
	marie := essai.Famille(t, srv)
	paul := essai.Parent(t, srv, marie, "Paul")
	auj := essai.Maintenant().Format("2006-01-02")
	marie.Get("/projets/nouveau")
	_, page := marie.Post("/projets", map[string]string{"nom": "Cuisine"})
	id := essai.Trouver(t, page, `action="/projets/(\d+)/etapes"`)
	if code, p := marie.Post("/suivi/projets/"+id+"/carnet", map[string]string{"libelle": "Peinture", "jour": auj, "montant": "120", "budget": "perso"}); code != 200 {
		t.Fatalf("carnet : %d %s", code, p[:300])
	}
	montant := func() (int64, int) {
		ops, _ := a.Q.OperationsPeriode(t.Context(), donnees.OperationsPeriodeParams{FamilleID: 1, Compte: 1, Du: "2000-01-01", Au: "2999-01-01"}) // Marie : membre 1
		if len(ops) == 0 {
			return 0, 0
		}
		return ops[0].Montant, len(ops)
	}
	if m, n := montant(); m != 12000 || n != 1 {
		t.Fatalf("dépense perso de Marie : %d (%d)", m, n)
	}
	_, page = paul.Get("/projets/" + id)
	ligne := essai.Trouver(t, page, `action="/suivi/carnet/(\d+)"`)
	paul.Post("/suivi/carnet/"+ligne, map[string]string{"libelle": "Peinture", "jour": auj, "montant": "999"})
	if m, _ := montant(); m != 12000 {
		t.Fatalf("Paul a changé le budget perso de Marie : %d", m)
	}
	// Marie vide le coût : la dépense quitte son budget.
	marie.Post("/suivi/carnet/"+ligne, map[string]string{"libelle": "Peinture", "jour": auj, "montant": ""})
	if _, n := montant(); n != 0 {
		t.Fatalf("coût vidé : %d dépense(s) restée(s) au budget", n)
	}
	// Nouvelle ligne perso de Marie, supprimée par Paul : la dépense reste.
	marie.Post("/suivi/projets/"+id+"/carnet", map[string]string{"libelle": "Évier", "jour": auj, "montant": "80", "budget": "perso"})
	_, page = paul.Get("/projets/" + id)
	for _, l := range regexp.MustCompile(`action="/suivi/carnet/(\d+)/supprimer"`).FindAllStringSubmatch(page, -1) {
		paul.Post("/suivi/carnet/"+l[1]+"/supprimer", nil)
	}
	if m, n := montant(); m != 8000 || n != 1 {
		t.Fatalf("Paul a supprimé la dépense perso de Marie : %d (%d)", m, n)
	}
}
