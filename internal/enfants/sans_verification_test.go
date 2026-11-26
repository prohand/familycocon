// SPDX-License-Identifier: GPL-3.0-or-later

package enfants

import (
	"strconv"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

// Mission « Pas besoin de vérifier » : cochée par l'enfant, ses étoiles
// sont gagnées tout de suite, sans passer par « À valider ».
func TestMissionSansVerification(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	membres, _ := a.Q.MembresDeFamille(t.Context(), 1)
	idLea := ""
	for _, m := range membres {
		if m.Nom == "Léa" {
			idLea = strconv.FormatInt(m.ID, 10)
		}
	}
	marie.Get("/enfants/missions/nouvelle")
	if _, p := marie.Get("/enfants/missions/nouvelle"); !strings.Contains(p, `name="sans_verification"`) {
		t.Fatal("case « Pas besoin de vérifier » absente")
	}
	marie.Post("/enfants/missions", map[string]string{"enfant": idLea, "titre": "Se brosser les dents", "etoiles": "2", "categorie": "routine", "repetition": "jour", "sans_verification": "1"})
	marie.Post("/enfants/missions", map[string]string{"enfant": idLea, "titre": "Ranger sa chambre", "etoiles": "3", "categorie": "menage", "repetition": "jour"})

	_, p := lea.Get("/enfants")
	idDents := essai.Trouver(t, p, `(?s)Se brosser les dents.*?/enfants/missions/(\d+)/faire`)
	idChambre := essai.Trouver(t, p, `(?s)Ranger sa chambre.*?/enfants/missions/(\d+)/faire`)
	lea.Htmx = true
	code, frag := lea.Post("/enfants/missions/"+idDents+"/faire", nil)
	if code != 200 || !strings.Contains(frag, "🌟 +2 étoiles !") || !strings.Contains(frag, "<b>2</b>") {
		t.Fatalf("mission sans vérification : %d\n%s", code, frag)
	}
	if !strings.Contains(lea.EnTetes.Get("HX-Trigger"), "Bravo") {
		t.Fatalf("message : %s", lea.EnTetes.Get("HX-Trigger"))
	}
	// L'autre mission attend toujours un parent.
	if _, frag = lea.Post("/enfants/missions/"+idChambre+"/faire", nil); !strings.Contains(frag, "Un parent vérifie") {
		t.Fatal("mission ordinaire validée sans parent")
	}
	// Chez le parent : une seule mission à valider ; les étoiles comptent
	// dans la semaine.
	_, p = marie.Get("/enfants")
	if strings.Count(p, "/enfants/validations/") != 2 { // valider, refuser
		t.Fatalf("à valider :\n%s", p)
	}
	if _, p = marie.Get("/enfants/enfant/" + idLea); !strings.Contains(p, "🌟 +2 étoiles !") || !strings.Contains(p, "<b>2</b>") {
		t.Fatal("espace de Léa vu par le parent")
	}
	// Décochée par le parent : la mission redevient à vérifier.
	if code, _ := marie.Post("/enfants/missions/"+idDents, map[string]string{"enfant": idLea, "titre": "Se brosser les dents", "etoiles": "2", "categorie": "routine", "repetition": "jour"}); code != 200 {
		t.Fatalf("modification : %d", code)
	}
	var sv int64
	if err := a.DB.QueryRow(`SELECT sans_verification FROM missions WHERE id = ?`, idDents).Scan(&sv); err != nil || sv != 0 {
		t.Fatalf("réglage non retiré : %d %v", sv, err)
	}
}
