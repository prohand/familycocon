// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

func TestDeplier(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	// Piscine le mercredi 14 octobre 2026 à 10:00 (heure d'été), 1 h, chaque semaine.
	d := time.Date(2026, 10, 14, 10, 0, 0, 0, paris)
	e := donnees.Evenement{Debut: d.Unix(), Fin: d.Add(time.Hour).Unix(), Repetition: "semaine"}
	debut := time.Date(2026, 11, 16, 0, 0, 0, 0, paris)
	occ := deplier(e, paris, debut, debut.AddDate(0, 0, 7))
	if len(occ) != 1 {
		t.Fatalf("%d occurrences", len(occ))
	}
	// Après le passage à l'heure d'hiver : toujours 10:00, un mercredi.
	if o := occ[0][0]; o.Hour() != 10 || o.Weekday() != time.Wednesday || o.Day() != 18 {
		t.Fatalf("occurrence %v", o)
	}
	// Avant le début : rien.
	if len(deplier(e, paris, d.AddDate(0, 0, -14), d.AddDate(0, 0, -7))) != 0 {
		t.Fatal("occurrence avant le début")
	}
	// Anniversaire : chaque année.
	anniv := time.Date(2018, 3, 5, 0, 0, 0, 0, paris)
	a := donnees.Evenement{Debut: anniv.Unix(), Fin: anniv.AddDate(0, 0, 1).Add(-time.Second).Unix(), Journee: 1, Repetition: "an"}
	mars := time.Date(2027, 3, 1, 0, 0, 0, 0, paris)
	if occ := deplier(a, paris, mars, mars.AddDate(0, 1, 0)); len(occ) != 1 || occ[0][0].Year() != 2027 || occ[0][0].Day() != 5 {
		t.Fatalf("anniversaire : %v", occ)
	}
	// Vacances sur plusieurs jours : visibles chaque jour.
	v := donnees.Evenement{Debut: time.Date(2026, 12, 19, 0, 0, 0, 0, paris).Unix(), Fin: time.Date(2027, 1, 4, 0, 0, 0, 0, paris).Add(-time.Second).Unix(), Journee: 1}
	noel := time.Date(2026, 12, 25, 0, 0, 0, 0, paris)
	if len(deplier(v, paris, noel, noel.AddDate(0, 0, 1))) != 1 {
		t.Fatal("vacances absentes le 25")
	}
	if len(deplier(v, paris, time.Date(2027, 1, 4, 0, 0, 0, 0, paris), time.Date(2027, 1, 5, 0, 0, 0, 0, paris))) != 0 {
		t.Fatal("vacances le 4 janvier (fin exclue)")
	}
}

func TestAgenda(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	// Un événement aujourd'hui (heure de Paris) pour Léa.
	paris, _ := time.LoadLocation("Europe/Paris")
	jour := time.Now().In(paris).Format("2006-01-02")
	marie.Get("/agenda/nouveau")
	membres, _ := a.Q.MembresDeFamille(t.Context(), 1)
	idLea := ""
	for _, m := range membres {
		if m.Nom == "Léa" {
			idLea = strconv.FormatInt(m.ID, 10)
		}
	}
	code, page := marie.Post("/agenda", map[string]string{
		"titre": "Piscine", "date": jour, "heure_debut": "10:00", "heure_fin": "11:00",
		"membre_" + idLea: "1", "repetition": "semaine", "lieu": "Centre aquatique",
	})
	if code != 200 || !strings.Contains(page, "Piscine") || !strings.Contains(page, `pastille c`) || !strings.Contains(page, "Centre aquatique") {
		t.Fatalf("création : %d", code)
	}
	if code, _ := marie.Post("/agenda", map[string]string{"titre": "X", "date": jour, "heure_debut": "10:00", "heure_fin": "09:00"}); code != 400 {
		t.Fatalf("fin avant le début acceptée : %d", code)
	}
	if code, _ := marie.Post("/agenda", map[string]string{"titre": "X", "date": "32/13/2026"}); code != 400 {
		t.Fatalf("date invalide acceptée : %d", code)
	}
	// Accueil : bloc du jour.
	if _, page := marie.Get("/"); !strings.Contains(page, "1 événement aujourd&#39;hui") || !strings.Contains(page, "Piscine") {
		t.Fatal("accueil sans l'événement du jour")
	}
	// Vue mois, et le mois suivant (répétition hebdomadaire).
	if _, page := marie.Get("/agenda?vue=mois"); !strings.Contains(page, `class="calendrier"`) || !strings.Contains(page, `evt-mini`) {
		t.Fatal("vue mois")
	}
	suivant := time.Now().In(paris).AddDate(0, 0, 35).Format("2006-01-02")
	if _, page := marie.Get("/agenda?vue=semaine&date=" + suivant); !strings.Contains(page, "Piscine") {
		t.Fatal("répétition absente 5 semaines plus tard")
	}
	// Léa ne modifie ni ne supprime l'événement de Marie.
	id := essai.Trouver(t, page, `href="/agenda/(\d+)[?"]`)
	if _, page := lea.Get("/agenda/" + id); !strings.Contains(page, "disabled") {
		t.Fatal("formulaire modifiable par un enfant")
	}
	if code, _ := lea.Post("/agenda/"+id, map[string]string{"titre": "Rien", "date": jour}); code != http.StatusForbidden {
		t.Fatalf("modification par un enfant : %d", code)
	}
	if code, _ := lea.Post("/agenda/"+id+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression par un enfant : %d", code)
	}
	marie.Get("/agenda/" + id)
	if code, page := marie.Post("/agenda/"+id+"/supprimer", nil); code != 200 || !strings.Contains(page, "supprimé") {
		t.Fatalf("suppression : %d", code)
	}
}

// Journée entière, chaque jour, le jour du passage à l'heure d'hiver (25 h
// à Paris) : acceptée (la durée permise a une heure de marge).
func TestJourneeChangementDHeure(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Get("/agenda/nouveau")
	if code, _ := marie.Post("/agenda", map[string]string{"titre": "Vacances", "date": "2026-10-25", "journee": "1", "repetition": "jour"}); code != 200 {
		t.Fatalf("journée entière répétée le jour du changement d'heure : %d", code)
	}
}
