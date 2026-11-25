// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/essai"
)

// Deux budgets : le compte commun (les deux parents) et le budget perso
// (privé) ; virements perso → commun, une fois ou chaque mois.
func TestBudgetPerso(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	paul := essai.Parent(t, srv, marie, "Paul")
	mois := time.Now().Format("2006-01")
	jour := time.Now().Format("2006-01-02")

	// Commun (comme avant) et perso de Marie.
	marie.Post("/budget", map[string]string{"montant": "100", "categorie": "courses", "libelle": "Marché commun", "jour": jour})
	marie.Post("/budget", map[string]string{"compte": "perso", "montant": "40", "categorie": "loisirs", "libelle": "Cinéma de Marie", "jour": jour})
	_, commun := marie.Get("/budget?compte=commun&mois=" + mois)
	if !strings.Contains(commun, "Marché commun") || strings.Contains(commun, "Cinéma de Marie") {
		t.Fatal("compte commun : mauvaises opérations")
	}
	_, perso := marie.Get("/budget?compte=perso&mois=" + mois)
	if !strings.Contains(perso, "Cinéma de Marie") || strings.Contains(perso, "Marché commun") || !strings.Contains(perso, "vous seul le voyez") {
		t.Fatal("budget perso : mauvaises opérations")
	}
	// Paul voit le commun, pas le perso de Marie (ni par son numéro).
	_, p := paul.Get("/budget?compte=commun&mois=" + mois)
	if !strings.Contains(p, "Marché commun") || strings.Contains(p, "Cinéma de Marie") {
		t.Fatal("Paul : compte commun")
	}
	if _, p = paul.Get("/budget?compte=perso&mois=" + mois); strings.Contains(p, "Cinéma de Marie") {
		t.Fatal("Paul voit le budget perso de Marie")
	}
	id := essai.Trouver(t, perso, `href="/budget/(\d+)"`)
	if code, _ := paul.Get("/budget/" + id); code != http.StatusNotFound {
		t.Fatalf("opération perso ouverte par Paul : %d", code)
	}
	if code, _ := paul.Post("/budget/"+id+"/supprimer", nil); code != http.StatusNotFound {
		t.Fatalf("opération perso supprimée par Paul : %d", code)
	}
	if _, p = paul.Get("/chercher?q=cinema"); strings.Contains(p, "Cinéma de Marie") {
		t.Fatal("recherche : le perso de Marie trouvé par Paul")
	}
	if _, p = marie.Get("/chercher?q=cinema"); !strings.Contains(p, "Cinéma de Marie") {
		t.Fatal("recherche : Marie ne trouve pas son opération perso")
	}

	// Virement de Marie vers le commun : dépense perso, revenu commun.
	marie.Post("/budget/virement", map[string]string{"montant": "500", "jour": jour})
	_, perso = marie.Get("/budget?compte=perso&mois=" + mois)
	_, p = paul.Get("/budget?compte=commun&mois=" + mois)
	if !strings.Contains(perso, `<span class="quoi">Virement vers le commun`) || !strings.Contains(p, "Virement de Marie") {
		t.Fatal("virement : une moitié manque")
	}
	// Supprimé du côté commun : les deux moitiés partent.
	vir := essai.Trouver(t, p, `href="/budget/(\d+)"[^>]*>\s*<span class="p-emoji">🔁`)
	if _, page := paul.Post("/budget/"+vir+"/supprimer", nil); !strings.Contains(page, "des deux côtés") {
		t.Fatal("virement non supprimé")
	}
	if _, perso = marie.Get("/budget?compte=perso&mois=" + mois); strings.Contains(perso, `<span class="quoi">Virement vers le commun`) {
		t.Fatal("virement : la moitié perso est restée")
	}

	// Virement chaque mois, commencé il y a 2 mois : 3 occurrences de chaque côté.
	debut := time.Now().AddDate(0, -2, 0).Format("2006-01-02")
	if code, _ := marie.Post("/budget/virement", map[string]string{"montant": "800", "jour": debut, "frequence": "mois"}); code != 200 {
		t.Fatalf("virement mensuel : %d", code)
	}
	for _, m := range []string{time.Now().AddDate(0, -2, 0).Format("2006-01"), mois} {
		_, perso = marie.Get("/budget?compte=perso&mois=" + m)
		_, p = paul.Get("/budget?compte=commun&mois=" + m)
		if !strings.Contains(perso, "800,00") || !strings.Contains(p, "Virement de Marie") || !strings.Contains(p, "800,00") {
			t.Fatalf("virement mensuel, mois %s", m)
		}
	}
	// La récurrence est dans le perso de Marie ; côté commun, c'est un
	// revenu qui revient (compté dans les revenus du mois), sans lien vers
	// le budget perso de Marie.
	if _, r := paul.Get("/budget/recurrences?compte=commun"); !strings.Contains(r, "Virement de Marie") ||
		!strings.Contains(r, `class="montant revenu">800,00`) || !strings.Contains(r, "Revenus <b>800,00") || strings.Contains(r, `href="/recurrences/`) {
		t.Fatal("virement mensuel : pas compté en revenu dans les récurrences du commun")
	}
	_, rp := marie.Get("/budget/recurrences?compte=perso")
	if !strings.Contains(rp, "800,00") {
		t.Fatal("virement mensuel absent des récurrences perso")
	}
	// Sa fiche ne demande pas de catégorie ; on change le montant sans en choisir.
	rid := essai.Trouver(t, rp, `href="/recurrences/(\d+)"`)
	_, fiche := marie.Get("/recurrences/" + rid)
	if strings.Contains(fiche, `type="radio" name="categorie"`) || !strings.Contains(fiche, `type="hidden" name="categorie" value="virement"`) {
		t.Fatal("virement qui revient : catégorie à choisir")
	}
	prochaine := essai.Trouver(t, fiche, `name="prochaine" value="([0-9-]+)"`)
	if code, _ := marie.Post("/recurrences/"+rid, map[string]string{"montant": "850", "categorie": "virement", "frequence": "mois",
		"prochaine": prochaine}); code != http.StatusOK {
		t.Fatalf("virement qui revient non modifié : %d", code)
	}
	// Le mois prochain : rien de passé, tout est prévu ; le reste à vivre
	// du commun compte le virement à venir.
	suivant := time.Now().AddDate(0, 1, 0).Format("2006-01")
	if time.Now().Day() > 28 { // 31 janvier + 1 mois : on vise le mois d'après le 1er
		suivant = time.Date(time.Now().Year(), time.Now().Month()+1, 1, 0, 0, 0, 0, time.Local).Format("2006-01")
	}
	_, p = paul.Get("/budget?compte=commun&mois=" + suivant)
	if !strings.Contains(p, "Revenus <b>850,00") || !strings.Contains(p, "dont 850,00") || !strings.Contains(p, "prévu compris") {
		t.Fatal("reste à vivre : le virement à venir n'est pas compté")
	}

	// Plafonds séparés.
	marie.Post("/budget/plafonds", map[string]string{"compte": "perso", "plafond_loisirs": "50"})
	if _, pl := marie.Get("/budget/plafonds?compte=perso"); !strings.Contains(pl, `value="50,00"`) {
		t.Fatal("plafond perso")
	}
	if _, pl := marie.Get("/budget/plafonds?compte=commun"); strings.Contains(pl, `value="50,00"`) {
		t.Fatal("plafond perso visible dans le commun")
	}
	// Accueil : les deux.
	if _, a := marie.Get("/"); !strings.Contains(a, "Commun :") || !strings.Contains(a, "Moi :") {
		t.Fatal("résumé de l'accueil")
	}
	// Le cookie retient le dernier budget ouvert : « Nouvelle dépense » sans
	// paramètre va dans le perso.
	marie.Get("/budget?compte=perso")
	if _, f := marie.Get("/budget/nouvelle"); !strings.Contains(f, `name="compte" value="perso"`) {
		t.Fatal("budget retenu par le cookie")
	}
}
