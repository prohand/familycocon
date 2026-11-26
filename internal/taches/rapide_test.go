// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
	"familycocon/internal/repetition"
)

// Filtres repliés : fermés par défaut, ouverts quand un filtre vient d'être
// choisi ; le membre gardé par l'appareil est rappelé dans le résumé.
// Ajout rapide : une fois, pour le membre et dans la catégorie filtrés.
func TestFiltresRepliesEtAjoutRapide(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	ctx := t.Context()
	membres, _ := a.Q.MembresDeFamille(ctx, 1)
	var idLea int64
	for _, mb := range membres {
		if mb.Nom == "Léa" {
			idLea = mb.ID
		}
	}
	lea.Post("/taches/categories", map[string]string{"nom": "Ménage", "emoji": "🧹"})
	_, p := marie.Get("/taches/categories")
	menage := essai.Trouver(t, p, `name="id" value="(\d+)"><input type="hidden" name="action" value="modifier">`)

	// Sans filtre : bloc fermé, pas de résumé, champ d'ajout rapide.
	_, p = marie.Get("/taches")
	if !strings.Contains(p, `<details class="filtres-taches bloc">`) || !strings.Contains(p, `<summary class="sous-titre">🔎 Filtrer</summary>`) {
		t.Fatalf("filtres pas repliés :\n%s", p)
	}
	if !strings.Contains(p, `placeholder="Ajouter une tâche…"`) || !strings.Contains(p, `href="/taches/nouvelle"`) {
		t.Fatal("ajout rapide ou formulaire complet absent")
	}
	// Catégorie choisie : ouvert, avec son nom dans le résumé.
	if _, p = marie.Get("/taches?pour=0&cat=" + menage); !strings.Contains(p, `<details class="filtres-taches bloc" open>`) || !strings.Contains(p, "<small>· 🧹 Ménage</small>") {
		t.Fatalf("filtre de catégorie :\n%s", p)
	}
	// Membre choisi : ouvert ; en revenant sans filtre dans l'adresse, le
	// membre est gardé (cookie) : fermé, rappelé dans le résumé.
	pour := strconv.FormatInt(idLea, 10)
	if _, p = marie.Get("/taches?pour=" + pour); !strings.Contains(p, "filtres-taches bloc\" open") || !strings.Contains(p, "<small>· Léa</small>") {
		t.Fatal("filtre du membre")
	}
	_, p = marie.Get("/taches")
	if !strings.Contains(p, `<details class="filtres-taches bloc">`) || !strings.Contains(p, "<small>· Léa</small>") || !strings.Contains(p, "Ajouter une tâche pour Léa…") {
		t.Fatalf("membre gardé par l'appareil :\n%s", p)
	}

	// Ajout rapide (htmx) : la liste revient avec la tâche.
	marie.Htmx = true
	code, frag := marie.Post("/taches/rapide", map[string]string{"titre": "Passer l'aspirateur", "pour": pour, "cat": menage})
	if code != 200 || !strings.Contains(frag, `id="taches"`) || !strings.Contains(frag, "Passer l&#39;aspirateur") {
		t.Fatalf("ajout rapide : %d\n%s", code, frag)
	}
	if code, _ := marie.Post("/taches/rapide", map[string]string{"titre": " "}); code != http.StatusBadRequest {
		t.Fatalf("titre vide : %d", code)
	}
	// Un membre d'ailleurs, une catégorie inconnue : tout le monde, sans catégorie.
	marie.Post("/taches/rapide", map[string]string{"titre": "Arroser", "pour": "999", "cat": "999"})
	taches, err := a.Q.TachesActives(ctx, donnees.TachesActivesParams{FamilleID: 1, MembreID: sql.NullInt64{Int64: 1, Valid: true}, Parent: 1})
	if err != nil || len(taches) != 2 {
		t.Fatalf("tâches : %d %v", len(taches), err)
	}
	cat, _ := strconv.ParseInt(menage, 10, 64)
	if x := taches[0]; x.AssigneA != (sql.NullInt64{Int64: idLea, Valid: true}) || x.CategorieID != (sql.NullInt64{Int64: cat, Valid: true}) || x.Repetition != "" || x.Emoji != "🧹" {
		t.Fatalf("tâche rapide : %+v", x)
	}
	if x := taches[1]; x.AssigneA.Valid || x.CategorieID.Valid {
		t.Fatalf("membre ou catégorie d'ailleurs gardés : %+v", x)
	}
	// Sans htmx : retour à la liste filtrée.
	marie.Htmx = false
	if code, p := marie.Post("/taches/rapide", map[string]string{"titre": "Plier le linge", "pour": "0"}); code != 200 || !strings.Contains(p, "Plier le linge") {
		t.Fatalf("ajout rapide sans htmx : %d", code)
	}
}

// « ⏭️ Passer au suivant » : le tour du jour passe au membre suivant, la
// suite continue à partir de lui, les jours d'avant ne changent pas.
func TestPasserTour(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	navs := map[string]*essai.Navigateur{"Marie": marie, "Léa": essai.Enfant(t, srv, marie, "Léa"), "Tom": essai.Enfant(t, srv, marie, "Tom")}
	ctx := t.Context()
	membres, _ := a.Q.MembresDeFamille(ctx, 1)
	parID := map[int64]*donnees.Membre{}
	var tour []string
	for i := range membres {
		parID[membres[i].ID] = &membres[i]
		tour = append(tour, strconv.FormatInt(membres[i].ID, 10))
	}
	marie.Get("/taches/nouvelle")
	if code, _ := marie.PostValeurs("/taches", url.Values{"titre": {"Vaisselle"}, "repetition": {"jour"}, "assigne": {"tour"}, "tour": tour}); code != 200 {
		t.Fatalf("création : %d", code)
	}
	taches, _ := a.Q.TachesActives(ctx, donnees.TachesActivesParams{FamilleID: 1, MembreID: sql.NullInt64{Int64: 1, Valid: true}, Parent: 1})
	tache := taches[0]
	id := strconv.FormatInt(tache.ID, 10)
	regle := repetition.Lire(tache.Repetition)
	auj := essai.Maintenant()
	qui := func(d decalages, jours int) *donnees.Membre {
		return assigneLe(tache, regle, auj.AddDate(0, 0, jours), parID, d)
	}
	hier, avant, apres := qui(nil, -1), qui(nil, 0), qui(nil, 1)

	// Un enfant dont ce n'est pas le tour ne peut pas le passer.
	for nom, n := range navs {
		if nom == avant.Nom || nom == "Marie" {
			continue
		}
		n.Get("/taches")
		n.Htmx = true
		if code, _ := n.Post("/taches/"+id+"/passer", map[string]string{"de": strconv.FormatInt(avant.ID, 10)}); code != http.StatusForbidden {
			t.Fatalf("%s passe le tour d'un autre : %d", nom, code)
		}
		break
	}
	// Celui dont c'est le tour le passe : c'est au suivant (« apres »).
	n := navs[avant.Nom]
	n.Htmx = false
	if _, p := n.Get("/taches"); !strings.Contains(p, `<span class="passer-mot">Passer à </span>`+apres.Nom) {
		t.Fatalf("bouton absent pour %s :\n%s", avant.Nom, p)
	}
	n.Htmx = true
	code, frag := n.Post("/taches/"+id+"/passer", map[string]string{"de": strconv.FormatInt(avant.ID, 10), "pour": "0"})
	if code != 200 || !strings.Contains(frag, apres.Nom+" 🔄") || !strings.Contains(frag, "passé à "+apres.Nom) {
		t.Fatalf("passer : %d\n%s", code, frag)
	}
	// Double envoi : refusé, rien n'est passé deux fois.
	if code, _ := n.Post("/taches/"+id+"/passer", map[string]string{"de": strconv.FormatInt(avant.ID, 10)}); code != http.StatusConflict {
		t.Fatalf("double envoi : %d", code)
	}
	passes, _ := a.Q.ToursPassesTache(ctx, tache.ID)
	if len(passes) != 1 {
		t.Fatalf("tours passés : %v", passes)
	}
	d := decalages{tache.ID: passes}
	if qui(d, -1).ID != hier.ID || qui(d, 0).ID != apres.ID || qui(d, 1).ID != qui(nil, 2).ID {
		t.Fatalf("suite du tour : hier %s, aujourd'hui %s, demain %s", qui(d, -1).Nom, qui(d, 0).Nom, qui(d, 1).Nom)
	}
	// Faite : plus rien à passer.
	marie.Get("/taches")
	marie.Htmx = true
	marie.Post("/taches/"+id+"/faire", nil)
	if code, _ := marie.Post("/taches/"+id+"/passer", nil); code != http.StatusBadRequest {
		t.Fatalf("passer une tâche faite : %d", code)
	}
	// Tâche sans tour de rôle : refusé.
	marie.Htmx = false
	marie.Post("/taches", map[string]string{"titre": "Arroser", "repetition": "jour"})
	_, p := marie.Get("/taches")
	autre := essai.Trouver(t, p, `/taches/(\d+)/faire"[^>]*Arroser`)
	marie.Htmx = true
	if code, _ := marie.Post("/taches/"+autre+"/passer", nil); code != http.StatusBadRequest {
		t.Fatalf("passer sans tour : %d", code)
	}
}
