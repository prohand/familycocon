// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
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
