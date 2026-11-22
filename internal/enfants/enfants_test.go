// SPDX-License-Identifier: GPL-3.0-or-later

package enfants

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"familycocon/internal/essai"
)

func TestParcoursComplet(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	membres, _ := a.Q.MembresDeFamille(t.Context(), 1)
	idLea := ""
	for _, m := range membres {
		if m.Nom == "Léa" {
			idLea = strconv.FormatInt(m.ID, 10)
			if m.Raccourcis != "accueil,enfants,taches,moi" {
				t.Errorf("barre par défaut d'un enfant : %s", m.Raccourcis)
			}
		}
	}

	// Marie crée une mission (3 étoiles, chaque jour) et une récompense (5 étoiles).
	marie.Get("/enfants/missions/nouvelle")
	if code, _ := marie.Post("/enfants/missions", map[string]string{"enfant": idLea, "titre": "Ranger sa chambre", "etoiles": "6", "categorie": "menage", "repetition": "jour"}); code != 400 {
		t.Fatalf("6 étoiles acceptées : %d", code)
	}
	code, page := marie.Post("/enfants/missions", map[string]string{"enfant": idLea, "titre": "Ranger sa chambre", "etoiles": "3", "categorie": "menage", "repetition": "jour"})
	if code != 200 || !strings.Contains(page, "Espace de Léa") || !strings.Contains(page, "⭐⭐⭐") {
		t.Fatalf("mission : %d", code)
	}
	marie.Get("/enfants/recompenses/nouvelle")
	marie.Post("/enfants/recompenses", map[string]string{"titre": "Soirée film", "prix": "5", "emoji": "🎬"})

	// Léa voit sa mission et coche.
	_, page = lea.Get("/enfants")
	if !strings.Contains(page, "Coucou Léa") || !strings.Contains(page, "Ranger sa chambre") || !strings.Contains(page, "Encore 5 ⭐") {
		t.Fatalf("espace de Léa :\n%s", page)
	}
	idMission := essai.Trouver(t, page, `/enfants/missions/(\d+)/faire`)
	lea.Htmx = true
	if code, frag := lea.Post("/enfants/missions/"+idMission+"/faire", nil); code != 200 || !strings.Contains(frag, "Un parent vérifie") {
		t.Fatalf("coche : %d", code)
	}
	// Léa ne valide pas elle-même.
	marie.Htmx = false
	_, page = marie.Get("/enfants")
	if !strings.Contains(page, "À valider") {
		t.Fatal("rien à valider chez le parent")
	}
	idFaite := essai.Trouver(t, page, `/enfants/validations/(\d+)/valider`)
	if code, _ := lea.Post("/enfants/validations/"+idFaite+"/valider", nil); code != http.StatusForbidden {
		t.Fatalf("auto-validation : %d", code)
	}
	// Refus, puis Léa recommence, puis validation.
	marie.Htmx = true
	marie.Post("/enfants/validations/"+idFaite+"/refuser", nil)
	if _, frag := lea.Get("/enfants/contenu"); !strings.Contains(frag, "À refaire") {
		t.Fatal("refus non visible")
	}
	lea.Post("/enfants/missions/"+idMission+"/faire", nil)
	if _, frag := marie.Post("/enfants/validations/"+idFaite+"/valider", nil); strings.Contains(frag, "À valider") {
		t.Fatal("toujours à valider après validation")
	}
	_, frag := lea.Get("/enfants/contenu")
	if !strings.Contains(frag, "juste-validee") || !strings.Contains(frag, "<b>3</b>") {
		t.Fatalf("étoiles non gagnées :\n%s", frag)
	}
	// Valider deux fois ne donne pas deux fois les étoiles.
	marie.Post("/enfants/validations/"+idFaite+"/valider", nil)
	if _, frag := lea.Get("/enfants/contenu"); !strings.Contains(frag, "<b>3</b>") {
		t.Fatal("double validation")
	}

	// 3 étoiles < 5 : demande refusée.
	idRec := essai.Trouver(t, page, `/enfants/recompenses/(\d+)"`)
	if code, _ := lea.Post("/enfants/recompenses/"+idRec+"/demander", nil); code != 400 {
		t.Fatalf("demande sans assez d'étoiles : %d", code)
	}
	// Une deuxième mission validée : 3 + 2 = 5.
	marie.Htmx = false
	marie.Get("/enfants/missions/nouvelle")
	marie.Post("/enfants/missions", map[string]string{"enfant": idLea, "titre": "Devoirs de maths", "etoiles": "2", "categorie": "devoirs", "repetition": "unique"})
	_, frag = lea.Get("/enfants/contenu")
	id2 := regexp.MustCompile(`/enfants/missions/(\d+)/faire`).FindStringSubmatch(frag)[1]
	lea.Post("/enfants/missions/"+id2+"/faire", nil)
	_, page = marie.Get("/enfants")
	marie.Htmx = true
	marie.Post("/enfants/validations/"+essai.Trouver(t, page, `/enfants/validations/(\d+)/valider`)+"/valider", nil)

	// Deux demandes en même temps : une seule passe (5 étoiles pour un prix de 5).
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = lea.Post("/enfants/recompenses/"+idRec+"/demander", nil)
		}(i)
	}
	wg.Wait()
	if !(codes[0] == 200 && codes[1] == 400 || codes[0] == 400 && codes[1] == 200) {
		t.Fatalf("demandes simultanées : %v", codes)
	}
	_, frag = lea.Get("/enfants/contenu")
	if !strings.Contains(frag, "<b>0</b>") || !strings.Contains(frag, "Demandée") {
		t.Fatalf("étoiles non réservées :\n%s", frag)
	}
	// Le parent confirme.
	marie.Htmx = false
	_, page = marie.Get("/enfants")
	idEch := essai.Trouver(t, page, `/enfants/echanges/(\d+)/confirmer`)
	marie.Htmx = true
	marie.Post("/enfants/echanges/"+idEch+"/confirmer", nil)
	if _, frag = lea.Get("/enfants/contenu"); !strings.Contains(frag, "Obtenue 🎉") || !strings.Contains(frag, "Cette semaine") {
		t.Fatalf("échange confirmé absent de l'historique :\n%s", frag)
	}

	// Un enfant ne crée ni ne supprime de mission.
	lea.Htmx = false
	if code, _ := lea.Get("/enfants/missions/nouvelle"); code != http.StatusForbidden {
		t.Fatalf("formulaire de mission ouvert à un enfant : %d", code)
	}
	if code, _ := lea.Post("/enfants/missions/"+idMission+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression par un enfant : %d", code)
	}
}

func TestMissionDUnAutreEnfant(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	hugo := essai.Enfant(t, srv, marie, "Hugo")
	_, page := marie.Get("/enfants/missions/nouvelle")
	idLea := essai.Trouver(t, page, `name="enfant" value="(\d+)"`)
	marie.Post("/enfants/missions", map[string]string{"enfant": idLea, "titre": "Lire", "etoiles": "1", "categorie": "devoirs", "repetition": "jour"})
	_, page = lea.Get("/enfants")
	id := essai.Trouver(t, page, `/enfants/missions/(\d+)/faire`)
	hugo.Get("/enfants")
	if code, _ := hugo.Post("/enfants/missions/"+id+"/faire", nil); code != http.StatusForbidden {
		t.Fatalf("Hugo coche la mission de Léa : %d", code)
	}
}
