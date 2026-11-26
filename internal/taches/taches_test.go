// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
	"familycocon/internal/repetition"
)

func TestTachesDuJour(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Get("/taches/nouvelle")
	if code, page := marie.Post("/taches", map[string]string{"titre": "Sortir les poubelles", "repetition": "jour"}); code != 200 || !strings.Contains(page, "Sortir les poubelles") || !strings.Contains(page, "🗑️") {
		t.Fatalf("création : %d", code)
	}
	if code, _ := marie.Post("/taches", map[string]string{"titre": "Arroser", "repetition": "jours"}); code != 400 {
		t.Fatalf("« certains jours » sans jour accepté : %d", code)
	}
	if code, _ := marie.Post("/taches", map[string]string{"titre": "", "repetition": "jour"}); code != 400 {
		t.Fatalf("titre vide accepté : %d", code)
	}
	marie.Post("/taches", map[string]string{"titre": "Ranger le garage", "repetition": "unique"})

	// Accueil : bloc « Tâches du jour » et résumé.
	_, page := marie.Get("/")
	if !strings.Contains(page, "Tâches du jour") || !strings.Contains(page, "2 à faire") {
		t.Fatalf("accueil sans les tâches :\n%s", page)
	}

	_, page = marie.Get("/taches")
	id := essai.Trouver(t, page, `/taches/(\d+)/faire"[^>]*Sortir les poubelles`)
	marie.Htmx = true
	code, frag := marie.Post("/taches/"+id+"/faire", map[string]string{"pour": "0"})
	if code != 200 || !strings.Contains(frag, "par Marie") || !strings.Contains(frag, "Qui a fait quoi") {
		t.Fatalf("faire : %d\n%s", code, frag)
	}
	// Deux fois : pas de doublon dans l'historique.
	_, frag = marie.Post("/taches/"+id+"/faire", map[string]string{"pour": "0"})
	if strings.Count(frag, "Sortir les poubelles</span>") != 1 {
		t.Fatalf("doublon dans l'historique :\n%s", frag)
	}
	if !strings.Contains(frag, "🏆 Cette semaine") {
		t.Fatal("scores absents")
	}
	if _, frag = marie.Post("/taches/"+id+"/defaire", map[string]string{"pour": "0"}); strings.Contains(frag, "par Marie") {
		t.Fatal("décochage sans effet")
	}
	// Cochée depuis l'accueil : la ligne seule revient, cochée.
	marie.Htmx = false
	if _, page := marie.Get("/"); !strings.Contains(page, `hx-post="/taches/`+id+`/faire"`) {
		t.Fatal("pas de case à cocher dans le bloc de l'accueil")
	}
	marie.Htmx = true
	if code, frag := marie.Post("/taches/"+id+"/faire", map[string]string{"depuis": "accueil"}); code != 200 || !strings.Contains(frag, "Fait par Marie") || strings.Contains(frag, "Qui a fait quoi") {
		t.Fatalf("faite depuis l'accueil : %d\n%s", code, frag)
	}
}

func TestDroitsEnfant(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Get("/taches/nouvelle")
	marie.Post("/taches", map[string]string{"titre": "Mettre la table", "repetition": "jour"})
	_, page := marie.Get("/taches")
	id := essai.Trouver(t, page, `/taches/(\d+)/faire`)

	// Marie coche ; Léa ne peut pas décocher, ni supprimer, ni modifier.
	marie.Htmx = true
	marie.Post("/taches/"+id+"/faire", map[string]string{"pour": "0"})
	lea.Get("/taches")
	lea.Htmx = true
	if code, _ := lea.Post("/taches/"+id+"/defaire", nil); code != http.StatusForbidden {
		t.Fatalf("décochage par un enfant : %d", code)
	}
	if code, _ := lea.Post("/taches/"+id+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression par un enfant : %d", code)
	}
	if code, _ := lea.Post("/taches/"+id, map[string]string{"titre": "Rien", "repetition": "jour"}); code != http.StatusForbidden {
		t.Fatalf("modification par un enfant : %d", code)
	}
	// Léa crée sa propre tâche et peut la modifier.
	lea.Htmx = false
	lea.Get("/taches/nouvelle")
	if code, page := lea.Post("/taches", map[string]string{"titre": "Nourrir le chat", "repetition": "jours", "jour1": "1", "jour3": "1"}); code != 200 || !strings.Contains(page, "🐈") {
		t.Fatalf("création par un enfant : %d", code)
	}
	// Suppression par un parent : la tâche disparaît, l'historique reste.
	marie.Htmx = false
	marie.Get("/taches")
	if code, page := marie.Post("/taches/"+id+"/supprimer", nil); code != 200 || !strings.Contains(page, "supprimée") || !strings.Contains(page, "Mettre la table</span>") {
		t.Fatalf("suppression : %d", code)
	}
}

// Revue : tâche « une fois » faite un autre jour, scores au-delà de 100
// coches, double coche, tâche pas prévue aujourd'hui, membre retiré.
func TestRevueTaches(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	ctx := t.Context()
	marie.Get("/taches/nouvelle")
	marie.Post("/taches", map[string]string{"titre": "Ranger le garage", "repetition": "unique"})
	marie.Post("/taches", map[string]string{"titre": "Sortir les poubelles", "repetition": "jour"})
	_, page := marie.Get("/taches")
	idGarage := essai.Trouver(t, page, `/taches/(\d+)/faire"[^>]*Ranger le garage`)
	idPoubelles := essai.Trouver(t, page, `/taches/(\d+)/faire"[^>]*Sortir les poubelles`)

	// Double coche : une seule annonce « Bravo » (la seconde ne change rien).
	marie.Htmx = true
	marie.Post("/taches/"+idGarage+"/faire", nil)
	req, _ := http.NewRequest("POST", srv.URL+"/taches/"+idGarage+"/faire", nil)
	req.Header.Set("X-Jeton", marie.Jeton)
	req.Header.Set("HX-Request", "true")
	rep, err := marie.C.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rep.Body.Close()
	if rep.Header.Get("HX-Trigger") != "" {
		t.Fatal("seconde coche : « Bravo » annoncé deux fois")
	}
	marie.Htmx = false

	// Faite hier : la tâche « une fois » n'est plus chargée du tout.
	hier := time.Now().AddDate(0, 0, -30).Unix() // bien avant cette semaine
	if _, err := a.DB.Exec(`UPDATE taches_faites SET fait_le = ? WHERE tache_id = ?`, hier, idGarage); err != nil {
		t.Fatal(err)
	}
	actives, err := a.Q.TachesActives(ctx, donnees.TachesActivesParams{FamilleID: 1, Depuis: time.Now().Add(-12 * time.Hour).Unix()})
	if err != nil || len(actives) != 1 || actives[0].Titre != "Sortir les poubelles" {
		t.Fatalf("tâche « une fois » faite hier encore chargée : %v %v", actives, err)
	}

	// Scores : 150 coches cette semaine sont toutes comptées (l'historique
	// affiché, lui, s'arrête à 100 lignes).
	if _, err := a.DB.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 150)
		INSERT INTO taches_faites (tache_id, occurrence, fait_par, fait_le) SELECT ?, 'essai' || i, 1, ? FROM n`, idPoubelles, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, page = marie.Get("/taches"); !strings.Contains(page, ">150<") {
		t.Fatalf("score de 150 absent :\n%s", page)
	}

	// « Certains jours », un jour non prévu : pas de coche par envoi direct.
	autre := (int(time.Now().UTC().Weekday())+2)%7 + 1 // ni hier, ni aujourd'hui, ni demain (quel que soit le fuseau)
	marie.Post("/taches", map[string]string{"titre": "Arroser", "repetition": "jours", "jour" + strconv.Itoa(autre): "1"})
	_, page = marie.Get("/taches")
	var idArroser string
	if err := a.DB.QueryRow(`SELECT id FROM taches WHERE titre = 'Arroser'`).Scan(&idArroser); err != nil {
		t.Fatal(err)
	}
	if code, _ := marie.Post("/taches/"+idArroser+"/faire", nil); code != http.StatusBadRequest {
		t.Fatalf("tâche cochée un jour non prévu : %d", code)
	}

	// Membre retiré : ses tâches reviennent à tout le monde.
	lea := essai.Enfant(t, srv, marie, "Léa")
	_ = lea
	marie.Get("/taches/nouvelle")
	marie.Post("/taches", map[string]string{"titre": "Nourrir le chat", "repetition": "jour", "assigne": "2"})
	if err := a.Q.LibererTachesMembre(ctx, donnees.LibererTachesMembreParams{FamilleID: 1, AssigneA: sql.NullInt64{Int64: 2, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	var reste int
	if err := a.DB.QueryRow(`SELECT count(*) FROM taches WHERE assigne_a = 2`).Scan(&reste); err != nil || reste != 0 {
		t.Fatalf("tâches encore confiées au membre retiré : %d %v", reste, err)
	}
}

// À tour de rôle : la tâche du jour revient à un membre, celle du
// lendemain au suivant.
func TestTourDeRole(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	essai.Enfant(t, srv, marie, "Léa")
	ids := map[string]string{}
	membres, _ := a.Q.MembresDeFamille(t.Context(), 1)
	for _, m := range membres {
		ids[m.Nom] = strconv.FormatInt(m.ID, 10)
	}
	marie.Get("/taches/nouvelle")
	if code, _ := marie.PostValeurs("/taches", url.Values{"titre": {"Vaisselle"}, "repetition": {"jour"}, "assigne": {"tour"}, "tour": {ids["Marie"]}}); code != 400 {
		t.Fatalf("tour de rôle d'une seule personne accepté : %d", code)
	}
	if code, _ := marie.PostValeurs("/taches", url.Values{"titre": {"Vaisselle"}, "repetition": {"jour"}, "assigne": {"tour"}, "tour": {ids["Marie"], ids["Léa"]}}); code != 200 {
		t.Fatalf("création : %d", code)
	}
	tache, _ := a.Q.TachesActives(t.Context(), donnees.TachesActivesParams{FamilleID: 1})
	parID := map[int64]*donnees.Membre{}
	for i := range membres {
		parID[membres[i].ID] = &membres[i]
	}
	regle := repetition.Lire(tache[0].Repetition)
	auj := essai.Maintenant()
	a1, a2 := assigneLe(tache[0], regle, auj, parID, nil), assigneLe(tache[0], regle, auj.AddDate(0, 0, 1), parID, nil)
	if a1 == nil || a2 == nil || a1.ID == a2.ID {
		t.Fatalf("pas de relais d'un jour à l'autre : %v %v", a1, a2)
	}
	if _, page := marie.Get("/taches"); !strings.Contains(page, a1.Nom+" 🔄") {
		t.Fatalf("personne du jour absente :\n%s", page)
	}
}

// Catégories : créées par tous, filtre, choix dans le formulaire ; une
// catégorie supprimée laisse ses tâches.
func TestCategoriesTaches(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	if code, _ := lea.Post("/taches/categories", map[string]string{"nom": "Ménage", "emoji": "🧹"}); code != 200 {
		t.Fatalf("catégorie créée par un enfant : %d", code)
	}
	if code, _ := marie.Post("/taches/categories", map[string]string{"nom": " ", "emoji": "🧹"}); code != 400 {
		t.Fatalf("nom vide : %d", code)
	}
	_, p := marie.Get("/taches/categories")
	menage := essai.Trouver(t, p, `name="id" value="(\d+)"><input type="hidden" name="action" value="modifier">\s*<span class="champ-emoji"><input type="hidden" name="emoji" value="🧹">`)
	if !strings.Contains(p, `<template id="grille-emojis">`) {
		t.Fatal("grille des emojis absente")
	}
	// Formulaire venu du filtre : catégorie déjà choisie.
	if _, p = marie.Get("/taches/nouvelle?cat=" + menage); !strings.Contains(p, `<option value="`+menage+`" selected>🧹 Ménage</option>`) {
		t.Fatalf("catégorie non proposée :\n%s", p)
	}
	marie.Post("/taches", map[string]string{"titre": "Passer l'aspirateur", "repetition": "jour", "categorie": menage})
	marie.Post("/taches", map[string]string{"titre": "Arroser", "repetition": "jour"})
	marie.Post("/taches", map[string]string{"titre": "Intrus", "repetition": "jour", "categorie": "999"}) // inconnue : aucune
	_, p = marie.Get("/taches?cat=" + menage)
	if !strings.Contains(p, "Passer l&#39;aspirateur") || strings.Contains(p, "Arroser") || !strings.Contains(p, `<span class="etiquette-categorie">🧹 Ménage</span>`) {
		t.Fatalf("filtre Ménage :\n%s", p)
	}
	if _, p = marie.Get("/taches?pour=0&cat=sans"); !strings.Contains(p, "Arroser") || !strings.Contains(p, "Intrus") || strings.Contains(p, "aspirateur") {
		t.Fatal("filtre « Sans catégorie »")
	}
	// Supprimée : la tâche reste, sans catégorie.
	marie.Post("/taches/categories", map[string]string{"action": "supprimer", "id": menage})
	if _, p = marie.Get("/taches?pour=0&cat=sans"); !strings.Contains(p, "aspirateur") {
		t.Fatal("tâche perdue avec sa catégorie")
	}
}

// Rappel d'une tâche : heure lue, montrée, envoyée une fois par jour.
func TestRappelTaches(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	if code, _ := marie.Post("/taches", map[string]string{"titre": "Poubelles", "repetition": "jour", "rappel": "25:00"}); code != 400 {
		t.Fatalf("heure impossible acceptée : %d", code)
	}
	marie.Post("/taches", map[string]string{"titre": "Poubelles", "repetition": "jour", "rappel": "18:00"})
	marie.Post("/taches", map[string]string{"titre": "Garage", "repetition": "unique", "rappel": "09:30", "date_rappel": "2030-05-04"})
	_, p := marie.Get("/taches")
	if !strings.Contains(p, "⏰ 18:00") || !strings.Contains(p, "⏰ 04/05/2030 à 09:30") {
		t.Fatalf("rappel non montré :\n%s", p)
	}
	// Une date sur une tâche qui revient est ignorée.
	marie.Post("/taches", map[string]string{"titre": "Vaisselle", "repetition": "jour", "rappel": "20:00", "date_rappel": "2030-05-04"})
	l, err := a.Q.TachesAvecRappel(t.Context())
	if err != nil || len(l) != 3 {
		t.Fatalf("tâches avec rappel : %v %v", l, err)
	}
	for _, x := range l {
		if x.Titre == "Vaisselle" && x.DateRappel != "" {
			t.Fatal("date gardée sur une tâche qui revient")
		}
	}
	// Envoi : demain à 18:01 (heure de Paris), une seule fois.
	paris, _ := time.LoadLocation("Europe/Paris")
	d := time.Now().In(paris).AddDate(0, 0, 1)
	demain := time.Date(d.Year(), d.Month(), d.Day(), 18, 1, 0, 0, paris)
	m := &Module{a: a}
	m.envoyerRappels(t.Context(), demain)
	var poubelles int64
	for _, x := range l {
		if x.Titre == "Poubelles" {
			poubelles = x.ID
		}
	}
	if n, _ := a.Q.MarquerRappelTache(t.Context(), donnees.MarquerRappelTacheParams{TacheID: poubelles, Jour: repetition.CleJour(demain), EnvoyeLe: 1}); n != 0 {
		t.Fatal("rappel de 18:00 non envoyé")
	}
}

// rappelDu : à l'heure, pas avant, pas trop tard ; jour de la période.
func TestRappelDu(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	lundi := time.Date(2026, 11, 23, 0, 0, 0, 0, paris) // un lundi
	a := func(j, h, mn int) time.Time {
		return lundi.AddDate(0, 0, j).Add(time.Duration(h)*time.Hour + time.Duration(mn)*time.Minute)
	}
	cas := []struct {
		nom              string
		repetition, date string
		quand            time.Time
		du               bool
	}{
		{"chaque jour, à l'heure", "jour", "", a(0, 18, 0), true},
		{"chaque jour, 10 min après", "jour", "", a(2, 18, 10), true},
		{"chaque jour, avant", "jour", "", a(0, 17, 59), false},
		{"chaque jour, trop tard", "jour", "", a(0, 18, 20), false},
		{"semaine, lundi", "semaine", "", a(0, 18, 0), true},
		{"semaine, mardi", "semaine", "", a(1, 18, 0), false},
		{"mois, le 1er", "mois", "", time.Date(2026, 12, 1, 18, 0, 0, 0, paris), true},
		{"mois, le 2", "mois", "", time.Date(2026, 12, 2, 18, 0, 0, 0, paris), false},
		{"certains jours, mardi prévu", "jours:2", "", a(1, 18, 0), true},
		{"certains jours, mercredi", "jours:2", "", a(2, 18, 0), false},
		{"une fois sans date : chaque jour", "", "", a(3, 18, 0), true},
		{"une fois, le jour choisi", "", "2026-11-25", a(2, 18, 0), true},
		{"une fois, un autre jour", "", "2026-11-25", a(1, 18, 0), false},
	}
	for _, x := range cas {
		r := donnees.TachesAvecRappelRow{Repetition: x.repetition, Rappel: "18:00", DateRappel: x.date, CreeLe: lundi.AddDate(0, 0, -30).Unix()}
		if _, du := rappelDu(r, x.quand); du != x.du {
			t.Errorf("%s : %v, attendu %v", x.nom, du, x.du)
		}
	}
	// Tâche créée après l'heure du rappel : rien ce jour-là.
	r := donnees.TachesAvecRappelRow{Repetition: "jour", Rappel: "18:00", CreeLe: a(0, 18, 5).Unix()}
	if _, du := rappelDu(r, a(0, 18, 6)); du {
		t.Error("rappel pour une tâche créée après l'heure")
	}
}

// Tâches privées, « parents seulement » (tâche ou catégorie) et filtre « Voir ».
func TestTachesPriveesEtParents(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	_, p := marie.Get("/taches/nouvelle")
	idLea := essai.Trouver(t, p, `<option value="(\d+)">Léa</option>`)
	if !strings.Contains(p, `name="privee"`) || !strings.Contains(p, `name="parents"`) {
		t.Fatal("cases Privée / Parents seulement absentes")
	}
	marie.Post("/taches", map[string]string{"titre": "Cadeau surprise", "repetition": "jour", "privee": "1", "assigne": idLea})
	marie.Post("/taches", map[string]string{"titre": "Declarer les impots", "repetition": "jour", "parents": "1"})
	marie.Post("/taches", map[string]string{"titre": "Ranger la chambre", "repetition": "jour", "parents": "1", "assigne": idLea})
	marie.Post("/taches", map[string]string{"titre": "Mettre la table", "repetition": "jour"})
	marie.Post("/taches/categories", map[string]string{"nom": "Papiers", "emoji": "📄", "parents": "1"})
	_, p = marie.Get("/taches/categories")
	papiers := essai.Trouver(t, p, `name="id" value="(\d+)"><input type="hidden" name="action" value="modifier">`)
	marie.Post("/taches", map[string]string{"titre": "Assurance", "repetition": "jour", "categorie": papiers})

	_, p = marie.Get("/taches")
	cadeau := essai.Trouver(t, p, `href="/taches/(\d+)/modifier" aria-label="Modifier Cadeau surprise"`)
	if !strings.Contains(p, `<span title="Privée">🔒</span> Cadeau surprise`) || !strings.Contains(p, `<span title="Parents seulement">👥</span> Declarer les impots`) ||
		!strings.Contains(p, `<span title="Parents seulement">👥</span> Assurance`) {
		t.Fatalf("repères 🔒 / 👥 absents :\n%s", p)
	}
	// Léa : ni la tâche privée (même confiée à elle), ni celles des parents,
	// sauf celle qu'elle doit faire.
	_, p = lea.Get("/taches")
	for _, cache := range []string{"Cadeau surprise", "Declarer les impots", "Assurance", "Papiers"} {
		if strings.Contains(p, cache) {
			t.Fatalf("Léa voit « %s »", cache)
		}
	}
	if !strings.Contains(p, "Ranger la chambre") || !strings.Contains(p, "Mettre la table") {
		t.Fatal("Léa ne voit pas ses tâches")
	}
	if code, _ := lea.Get("/taches/" + cadeau + "/modifier"); code != http.StatusNotFound {
		t.Fatalf("tâche privée ouverte par Léa : %d", code)
	}
	if code, _ := lea.Post("/taches/"+cadeau+"/faire", nil); code != http.StatusNotFound {
		t.Fatalf("tâche privée cochée par Léa : %d", code)
	}
	if code, _ := lea.Post("/taches/categories", map[string]string{"action": "parents", "id": papiers, "parents": "0"}); code != http.StatusForbidden {
		t.Fatalf("réglage changé par un enfant : %d", code)
	}
	// Léa ne peut pas cacher sa propre tâche aux enfants (case ignorée).
	lea.Post("/taches", map[string]string{"titre": "Mon dessin", "repetition": "jour", "parents": "1"})
	if _, p = marie.Get("/taches?voir=enfants"); !strings.Contains(p, "Mon dessin") {
		t.Fatal("« parents seulement » posé par un enfant")
	}
	// Filtre « Vues des enfants » (parents).
	for _, x := range []struct {
		titre string
		vue   bool
	}{{"Ranger la chambre", true}, {"Mettre la table", true}, {"Declarer les impots", false}, {"Assurance", false}, {"Cadeau surprise", false}} {
		if strings.Contains(p, x.titre) != x.vue {
			t.Errorf("filtre enfants : « %s » montrée = %v", x.titre, !x.vue)
		}
	}
	// Nombres : 6 tâches (avec celle de Léa), 1 privée, 3 vues des enfants.
	if _, q := marie.Get("/taches"); !strings.Contains(q, "Toutes <small>6</small>") || !strings.Contains(q, "Privées <small>1</small>") ||
		!strings.Contains(q, "Vues des enfants <small>3</small>") {
		t.Fatalf("nombres du filtre « Voir » :\n%s", q)
	}
	if _, p = marie.Get("/taches?voir=privees"); !strings.Contains(p, "Cadeau surprise") || strings.Contains(p, "Mettre la table") {
		t.Fatal("filtre Privées")
	}
	if _, p = lea.Get("/taches"); strings.Contains(p, "voir=enfants") {
		t.Fatal("filtre « Vues des enfants » proposé à un enfant")
	}
	// Historique : une tâche des parents cochée n'apparaît pas chez Léa.
	_, p = marie.Get("/taches")
	impots := essai.Trouver(t, p, `href="/taches/(\d+)/modifier" aria-label="Modifier Declarer les impots"`)
	marie.Post("/taches/"+impots+"/faire", nil)
	if _, p = lea.Get("/taches"); strings.Contains(p, "impots") {
		t.Fatal("historique : tâche des parents montrée à un enfant")
	}
}

// Pièces jointes d'une tâche : montrées dans la liste, ajoutées par qui peut
// la modifier.
func TestJointsTaches(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Post("/taches", map[string]string{"titre": "Remplir le dossier", "repetition": "unique"})
	_, p := marie.Get("/taches")
	id := essai.Trouver(t, p, `href="/taches/(\d+)/modifier" aria-label="Modifier Remplir le dossier"`)
	if _, p = marie.Get("/taches/" + id + "/modifier"); !strings.Contains(p, `action="/joints/taches/`+id+`"`) {
		t.Fatal("bouton « Joindre un fichier » absent")
	}
	f := essai.Fichier{Champ: "fichier", Nom: "notice.pdf", Contenu: []byte("%PDF-1.4\nnotice")}
	if code, _ := marie.PostFichiers("/joints/taches/"+id, map[string]string{"retour": "/taches/" + id + "/modifier"}, f); code != 200 {
		t.Fatalf("envoi : %d", code)
	}
	if _, p = lea.Get("/taches"); !strings.Contains(p, "📎 notice.pdf") {
		t.Fatal("pièce absente de la liste")
	}
	if code, _ := lea.PostFichiers("/joints/taches/"+id, nil, f); code != http.StatusForbidden {
		t.Fatalf("pièce ajoutée par qui ne peut pas modifier : %d", code)
	}
}

// « Pour qui ? » : Parents (l'un ou l'autre).
func TestTachePourParents(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	_, p := marie.Get("/taches/nouvelle")
	if !strings.Contains(p, `<option value="parents">👥 Parents</option>`) {
		t.Fatal("choix « Parents » absent")
	}
	idMarie := essai.Trouver(t, p, `<option value="(\d+)">Marie</option>`)
	idLea := essai.Trouver(t, p, `<option value="(\d+)">Léa</option>`)
	marie.Post("/taches", map[string]string{"titre": "Payer la cantine", "repetition": "jour", "assigne": "parents"})
	_, p = marie.Get("/taches?pour=" + idMarie)
	if !strings.Contains(p, "Payer la cantine") || !strings.Contains(p, "Parents · Chaque jour") || !strings.Contains(p, `title="Parents">👥</span>`) {
		t.Fatalf("tâche des parents absente chez Marie :\n%s", p)
	}
	if _, p = lea.Get("/taches?pour=" + idLea); strings.Contains(p, "Payer la cantine") {
		t.Fatal("tâche des parents montrée dans le filtre de Léa")
	}
	id := essai.Trouver(t, p+mustGet(t, marie, "/taches"), `href="/taches/(\d+)/modifier" aria-label="Modifier Payer la cantine"`)
	if _, p = marie.Get("/taches/" + id + "/modifier"); !strings.Contains(p, `<option value="parents" selected>👥 Parents</option>`) {
		t.Fatal("choix « Parents » non gardé")
	}
}

func mustGet(t *testing.T, n *essai.Navigateur, chemin string) string {
	t.Helper()
	_, p := n.Get(chemin)
	return p
}

// Description facultative : montrée mise en forme, trouvée par la recherche.
func TestDescriptionTache(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Post("/taches", map[string]string{"titre": "Centre aéré", "repetition": "unique", "description": "Réserver sur le **site de la mairie**"})
	_, p := marie.Get("/taches")
	if !strings.Contains(p, `<details class="description-tache"><summary>Réserver sur le site de la mairie</summary>`) || !strings.Contains(p, "<strong>site de la mairie</strong>") {
		t.Fatalf("description absente :\n%s", p)
	}
	if code, _ := marie.Post("/taches", map[string]string{"titre": "X", "repetition": "jour", "description": strings.Repeat("a", 2001)}); code != 400 {
		t.Fatalf("description trop longue acceptée : %d", code)
	}
}

// Une tâche « une fois » datée plus tard n'est pas « du jour » (accueil) :
// elle attend dans « Les autres jours », et on peut déjà la cocher.
func TestTacheDateePlusTard(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	plusTard := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	marie.Post("/taches", map[string]string{"titre": "Reserver le centre", "repetition": "unique", "rappel": "12:30", "date_rappel": plusTard})
	marie.Post("/taches", map[string]string{"titre": "Sortir les poubelles", "repetition": "jour"})
	_, p := marie.Get("/")
	if strings.Contains(p, "Reserver le centre") || !strings.Contains(p, "Sortir les poubelles") {
		t.Fatalf("accueil : tâche datée plus tard montrée comme « du jour »")
	}
	_, p = marie.Get("/taches")
	i, j := strings.Index(p, "Les autres jours"), strings.Index(p, "Reserver le centre")
	if i < 0 || j < i {
		t.Fatal("tâche datée plus tard absente de « Les autres jours »")
	}
	if !strings.Contains(p[i:], `aria-label="J'ai fait Reserver le centre"`) {
		t.Fatal("tâche datée plus tard : pas de case à cocher")
	}
}
