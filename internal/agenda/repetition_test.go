// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

func TestReglesRepetition(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	jour := func(s string) time.Time { d, _ := time.ParseInLocation("2006-01-02 15:04", s, paris); return d }
	evt := func(debut, rep, fin string) donnees.Evenement {
		d := jour(debut)
		return donnees.Evenement{Debut: d.Unix(), Fin: d.Add(time.Hour).Unix(), Repetition: rep, RepetitionFin: fin}
	}
	cas := []struct {
		nom      string
		e        donnees.Evenement
		du, au   string
		attendus []string // jours des occurrences
	}{
		{"toutes les 2 semaines", evt("2026-11-04 10:00", "2semaines", ""), "2026-11-09 00:00", "2026-11-23 00:00", []string{"2026-11-18"}},
		{"mardi et jeudi", evt("2026-11-03 17:00", "jours:2,4", ""), "2026-11-16 00:00", "2026-11-23 00:00", []string{"2026-11-17", "2026-11-19"}},
		{"chaque jour jusqu'au 18", evt("2026-11-01 08:00", "jour", "2026-11-18"), "2026-11-16 00:00", "2026-11-23 00:00", []string{"2026-11-16", "2026-11-17", "2026-11-18"}},
		{"le 31 en février", evt("2026-01-31 09:00", "mois", ""), "2026-02-01 00:00", "2026-03-01 00:00", []string{"2026-02-28"}},
		{"2e mardi", evt("2026-11-10 20:00", "mois-rang", ""), "2026-12-01 00:00", "2027-01-01 00:00", []string{"2026-12-08"}},
		{"dernier mardi", evt("2026-12-29 20:00", "mois-rang", ""), "2027-01-01 00:00", "2027-02-01 00:00", []string{"2027-01-26"}},
		{"29 février", evt("2024-02-29 00:00", "an", ""), "2025-02-01 00:00", "2025-03-01 00:00", []string{"2025-02-28"}},
		{"pas avant le début", evt("2026-11-20 10:00", "jour", ""), "2026-11-16 00:00", "2026-11-23 00:00", []string{"2026-11-20", "2026-11-21", "2026-11-22"}},
		{"trimestriel", evt("2026-01-15 09:00", "trimestre", ""), "2026-01-01 00:00", "2027-01-01 00:00", []string{"2026-01-15", "2026-04-15", "2026-07-15", "2026-10-15"}},
		{"semestriel, le 31", evt("2026-08-31 09:00", "semestre", ""), "2026-08-01 00:00", "2027-09-01 00:00", []string{"2026-08-31", "2027-02-28", "2027-08-31"}},
		{"tous les 2 mois", evt("2026-11-05 09:00", "2mois", "2027-03-31"), "2026-11-01 00:00", "2027-12-01 00:00", []string{"2026-11-05", "2027-01-05", "2027-03-05"}},
		{"chaque semaine, heure d'hiver", evt("2026-10-14 10:00", "semaine", ""), "2026-11-16 00:00", "2026-11-23 00:00", []string{"2026-11-18"}},
	}
	for _, c := range cas {
		occ := deplier(c.e, paris, jour(c.du), jour(c.au))
		var jours []string
		for _, o := range occ {
			jours = append(jours, o[0].Format("2006-01-02"))
			if o[0].Hour() != time.Unix(c.e.Debut, 0).In(paris).Hour() {
				t.Errorf("%s : heure décalée %v", c.nom, o[0])
			}
		}
		if strings.Join(jours, " ") != strings.Join(c.attendus, " ") {
			t.Errorf("%s : %v, attendu %v", c.nom, jours, c.attendus)
		}
	}
	// Textes.
	if s := TexteRepetition(evt("2026-11-10 20:00", "mois-rang", "2027-06-30"), paris); s != "Chaque 2e mardi du mois, jusqu'au 30 juin 2027" {
		t.Errorf("texte : %q", s)
	}
	if s := TexteRepetition(evt("2026-01-15 09:00", "trimestre", ""), paris); s != "Tous les 3 mois, le 15" {
		t.Errorf("texte : %q", s)
	}
	if s := TexteRepetition(evt("2026-11-03 17:00", "jours:2,4", ""), paris); s != "Chaque mar, jeu" {
		t.Errorf("texte : %q", s)
	}
}

func TestFormulaireRepetition(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Get("/agenda/nouveau")
	base := map[string]string{"titre": "Judo", "date": "2026-11-17", "heure_debut": "17:00", "heure_fin": "18:00", "repetition": "jours"}
	if code, _ := marie.Post("/agenda", base); code != 400 {
		t.Fatalf("certains jours sans jour coché : %d", code)
	}
	base["rjour2"], base["rjour4"], base["repetition_fin"] = "1", "1", "2027-06-30"
	if code, _ := marie.Post("/agenda", base); code != 200 {
		t.Fatalf("création : %d", code)
	}
	_, page := marie.Get("/agenda?date=2026-11-16")
	if strings.Count(page, "Judo") != 2 {
		t.Fatalf("judo mardi et jeudi : %d fois", strings.Count(page, "Judo"))
	}
	id := essai.Trouver(t, page, `href="/agenda/(\d+)"`)
	if _, page := marie.Get("/agenda/" + id); !strings.Contains(page, "Chaque mar, jeu, jusqu&#39;au 30 juin 2027") {
		t.Fatalf("texte de la répétition absent :\n%s", page)
	}
	base["repetition_fin"] = "2026-01-01"
	if code, _ := marie.Post("/agenda", base); code != 400 {
		t.Fatalf("fin avant le début acceptée : %d", code)
	}
	if code, _ := marie.Post("/agenda", map[string]string{"titre": "X", "date": "2026-11-17", "repetition": "toutes-les-heures"}); code != 200 {
		t.Fatalf("répétition inconnue : %d (elle devient « une fois »)", code)
	}
}

// Journée entière qui revient un jour de passage à l'heure d'été (23 h) :
// elle ne déborde pas sur le lendemain.
func TestJourneeChangementHeure(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	d := time.Date(2027, 3, 21, 0, 0, 0, 0, paris)
	e := donnees.Evenement{Debut: d.Unix(), Fin: d.AddDate(0, 0, 1).Add(-time.Second).Unix(), Journee: 1, Repetition: "semaine"}
	lundi := time.Date(2027, 3, 29, 0, 0, 0, 0, paris)
	if occ := deplier(e, paris, lundi, lundi.AddDate(0, 0, 1)); len(occ) != 0 {
		t.Fatalf("déborde sur le lundi : %v", occ)
	}
	if occ := deplier(e, paris, lundi.AddDate(0, 0, -1), lundi); len(occ) != 1 {
		t.Fatalf("absent le dimanche 28 : %v", occ)
	}
}

// Événement démesuré : refusé à la saisie, et plafonné s'il existait déjà.
func TestAgendaDemesure(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Get("/agenda/nouveau")
	if code, _ := marie.Post("/agenda", map[string]string{"titre": "X", "date": "1800-01-01", "date_fin": "2090-01-01", "repetition": "jour"}); code != 400 {
		t.Fatalf("événement de 290 ans accepté : %d", code)
	}
	if code, _ := marie.Post("/agenda", map[string]string{"titre": "X", "date": "2026-01-01", "date_fin": "2026-03-01", "repetition": "semaine"}); code != 400 {
		t.Fatalf("événement de 2 mois chaque semaine accepté : %d", code)
	}
	paris, _ := time.LoadLocation("Europe/Paris")
	d := time.Date(1900, 1, 1, 0, 0, 0, 0, paris)
	e := donnees.Evenement{Debut: d.Unix(), Fin: time.Date(2090, 1, 1, 0, 0, 0, 0, paris).Unix(), Journee: 1, Repetition: "jour"}
	semaine := time.Date(2026, 11, 23, 0, 0, 0, 0, paris)
	if occ := deplier(e, paris, semaine, semaine.AddDate(0, 0, 7)); len(occ) > occurrencesMax {
		t.Fatalf("%d occurrences", len(occ))
	}
}
