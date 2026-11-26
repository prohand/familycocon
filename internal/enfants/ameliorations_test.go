// SPDX-License-Identifier: GPL-3.0-or-later

package enfants

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

// Mission pour deux enfants d'un coup, « Tout valider », étoiles bonus.
func TestMissionsGroupeesEtBonus(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	tom := essai.Enfant(t, srv, marie, "Tom")
	ids := map[string]string{}
	membres, _ := a.Q.MembresDeFamille(t.Context(), 1)
	for _, m := range membres {
		ids[m.Nom] = strconv.FormatInt(m.ID, 10)
	}
	marie.Get("/enfants/missions/nouvelle")
	code, _ := marie.PostValeurs("/enfants/missions", url.Values{"enfant": {ids["Léa"], ids["Tom"]}, "titre": {"Mettre la table"},
		"etoiles": {"2"}, "categorie": {"routine"}, "repetition": {"jour"}})
	if code != 200 {
		t.Fatalf("mission pour deux enfants : %d", code)
	}
	for _, n := range []*essai.Navigateur{lea, tom} {
		_, page := n.Get("/enfants")
		id := essai.Trouver(t, page, `/enfants/missions/(\d+)/faire`)
		n.Post("/enfants/missions/"+id+"/faire", nil)
	}
	_, page := marie.Get("/enfants")
	if !strings.Contains(page, "Valider les 2 missions") || !strings.Contains(page, `aria-label="Valider « Mettre la table »"`) {
		t.Fatalf("bouton « Tout valider » absent :\n%s", page)
	}
	marie.Htmx = true
	if code, frag := marie.Post("/enfants/validations/tout", nil); code != 200 || strings.Contains(frag, "À valider") {
		t.Fatalf("tout valider : %d", code)
	}
	marie.Htmx = false
	// Bonus : +3 pour Léa, puis −10 refusé (elle n'en a que 5).
	if code, _ := marie.Post("/enfants/enfant/"+ids["Léa"]+"/bonus", map[string]string{"etoiles": "3", "motif": "A aidé son frère"}); code != 200 {
		t.Fatalf("bonus : %d", code)
	}
	if code, _ := marie.Post("/enfants/enfant/"+ids["Léa"]+"/bonus", map[string]string{"etoiles": "-10", "motif": "Gros mot"}); code != 400 {
		t.Fatalf("retrait au-delà du solde accepté : %d", code)
	}
	if code, _ := marie.Post("/enfants/enfant/"+ids["Léa"]+"/bonus", map[string]string{"etoiles": "0", "motif": "x"}); code != 400 {
		t.Fatal("bonus de 0 accepté")
	}
	if code, _ := lea.Post("/enfants/enfant/"+ids["Léa"]+"/bonus", map[string]string{"etoiles": "5", "motif": "moi"}); code != 403 {
		t.Fatal("un enfant se donne des étoiles")
	}
	_, page = lea.Get("/enfants")
	if !strings.Contains(page, "A aidé son frère") || !strings.Contains(page, "<b>5</b> étoiles") {
		t.Fatalf("bonus absent de l'espace de Léa :\n%s", page)
	}
	// Les étoiles bonus comptent pour une récompense (2 de mission + 3 bonus).
	marie.Get("/enfants/recompenses/nouvelle")
	marie.Post("/enfants/recompenses", map[string]string{"titre": "Glace", "prix": "5", "emoji": "🍦"})
	_, page = lea.Get("/enfants")
	rec := essai.Trouver(t, page, `/enfants/recompenses/(\d+)/demander`)
	if code, _ := lea.Post("/enfants/recompenses/"+rec+"/demander", nil); code != 200 {
		t.Fatalf("récompense refusée malgré les étoiles bonus : %d", code)
	}
}
