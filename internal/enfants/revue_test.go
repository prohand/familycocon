// SPDX-License-Identifier: GPL-3.0-or-later

package enfants

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

// faire : comme htmx ; renvoie le code et l'en-tête HX-Trigger (la
// notification « Super ! »).
func faire(t *testing.T, n *essai.Navigateur, adresse string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", adresse, strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Jeton", n.Jeton)
	req.Header.Set("HX-Request", "true")
	rep, err := n.C.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	return rep.StatusCode, rep.Header.Get("HX-Trigger")
}

// Revue du 23/11/2026.
func TestRevueEnfants(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	ctx := context.Background()
	membres, _ := a.Q.MembresDeFamille(ctx, 1)
	var lea64 int64
	for _, m := range membres {
		if m.Nom == "Léa" {
			lea64 = m.ID
		}
	}
	idLea := strconv.FormatInt(lea64, 10)

	marie.Get("/enfants/missions/nouvelle")
	marie.Post("/enfants/missions", map[string]string{"enfant": idLea, "titre": "Devoirs de maths", "etoiles": "2", "categorie": "devoirs", "repetition": "unique"})
	_, frag := lea.Get("/enfants/contenu")
	id := regexp.MustCompile(`/enfants/missions/(\d+)/faire`).FindStringSubmatch(frag)[1]

	// Cochée deux fois : la seconde ne change rien et ne notifie personne.
	code, declencheur := faire(t, lea, srv.URL+"/enfants/missions/"+id+"/faire")
	if code != 200 || !strings.Contains(declencheur, "fc-toast") {
		t.Fatalf("première coche : %d %q", code, declencheur)
	}
	code, declencheur = faire(t, lea, srv.URL+"/enfants/missions/"+id+"/faire")
	if code != 200 || declencheur != "" {
		t.Fatalf("seconde coche : %d %q (notification en double)", code, declencheur)
	}
	// La requête dit si une ligne a changé, et vérifie la famille.
	mission, _ := strconv.ParseInt(id, 10, 64)
	if n, _ := a.Q.CocherMission(ctx, donnees.CocherMissionParams{MissionID: mission, FamilleID: 2, Occurrence: "autre", Etoiles: 2, CocheLe: 1}); n != 0 {
		t.Fatal("mission cochée par une autre famille")
	}
	attente, _ := a.Q.MissionsEnAttente(ctx, 1)
	if len(attente) != 1 {
		t.Fatalf("%d missions en attente", len(attente))
	}
	if n, _ := a.Q.DeciderMission(ctx, donnees.DeciderMissionParams{Etat: Validee, ID: attente[0].ID, FamilleID: 2}); n != 0 {
		t.Fatal("mission validée par une autre famille")
	}

	// Validée : les étoiles sont gagnées. Le lendemain, la mission « une
	// fois » est archivée (plus relue à chaque page) et les étoiles restent.
	_, page := marie.Get("/enfants")
	marie.Post("/enfants/validations/"+essai.Trouver(t, page, `/enfants/validations/(\d+)/valider`)+"/valider", nil)
	if actives, _ := a.Q.MissionsActives(ctx, 1); len(actives) != 1 {
		t.Fatalf("%d missions actives juste après la validation", len(actives))
	}
	if _, err := a.DB.ExecContext(ctx, `UPDATE missions_faites SET decide_le = ?`, time.Now().Add(-48*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	lea.Get("/enfants/contenu")
	if actives, _ := a.Q.MissionsActives(ctx, 1); len(actives) != 0 {
		t.Fatalf("mission finie toujours active : %d", len(actives))
	}
	if g, _ := a.Q.EtoilesGagnees(ctx, lea64); g != 2 {
		t.Fatalf("étoiles après archivage : %d", g)
	}

	// Récompense demandée deux fois de suite : une seule demande.
	marie.Get("/enfants/recompenses/nouvelle")
	marie.Post("/enfants/recompenses", map[string]string{"titre": "Glace", "emoji": "🍦", "prix": "1"})
	_, frag = lea.Get("/enfants/contenu")
	rec := regexp.MustCompile(`/enfants/recompenses/(\d+)/demander`).FindStringSubmatch(frag)[1]
	lea.Post("/enfants/recompenses/"+rec+"/demander", nil)
	lea.Post("/enfants/recompenses/"+rec+"/demander", nil)
	if e, _ := a.Q.EchangesEnAttente(ctx, 1); len(e) != 1 {
		t.Fatalf("%d demandes en attente", len(e))
	}
	// Décidée seulement par la famille.
	e, _ := a.Q.EchangesEnAttente(ctx, 1)
	if n, _ := a.Q.DeciderEchange(ctx, donnees.DeciderEchangeParams{Etat: "confirme", ID: e[0].ID, FamilleID: 2}); n != 0 {
		t.Fatal("échange confirmé par une autre famille")
	}
}

// Les index de la migration 0019 servent bien aux requêtes visées.
func TestRevueIndex(t *testing.T) {
	a, _ := essai.NouvelleApp(t, Brancher)
	plan := func(requete string, args ...any) string {
		lignes, err := a.DB.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+requete, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer lignes.Close()
		var out []string
		for lignes.Next() {
			var id, parent, inutile int
			var detail string
			if err := lignes.Scan(&id, &parent, &inutile, &detail); err != nil {
				t.Fatal(err)
			}
			out = append(out, detail)
		}
		return strings.Join(out, " | ")
	}
	for index, requete := range map[string]string{
		"missions_enfant": `SELECT sum(f.etoiles) FROM missions_faites f JOIN missions m ON m.id = f.mission_id
			WHERE m.enfant_id = 1 AND f.etat = 'validee'`,
		"missions_faites_attente": `SELECT f.id FROM missions_faites f JOIN missions m ON m.id = f.mission_id
			WHERE m.famille_id = 1 AND f.etat = 'attente'`,
		"recompenses_famille":         `SELECT e.id FROM echanges e JOIN recompenses r ON r.id = e.recompense_id WHERE r.famille_id = 1 AND e.etat = 'attente'`,
		"echanges_recompense":         `SELECT id FROM echanges WHERE recompense_id = 1`,
		"operations_recurrence":       `DELETE FROM operations WHERE recurrence_id = 1 AND famille_id = 1`,
		"abonnements_famille":         `SELECT id FROM abonnements WHERE famille_id = 1`,
		"evenements_contacts_contact": `SELECT evenement_id FROM evenements_contacts WHERE contact_id = 1`,
	} {
		p := plan(requete)
		t.Logf("%s : %s", index, p)
		if !strings.Contains(p, index) {
			t.Errorf("index %s non utilisé : %s", index, p)
		}
		if strings.Contains(p, "SCAN") {
			t.Errorf("parcours complet restant : %s", p)
		}
	}
}
