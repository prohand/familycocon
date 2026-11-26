// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

// Heure du rappel juste avant : en vraie durée (15 min, 1 h), la veille à
// 18 h d'horloge, même autour du changement d'heure.
func TestHeureRappelAvant(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	// Dimanche 28 mars 2027 : à 2 h il est 3 h (heure d'été).
	d := time.Date(2027, 3, 28, 3, 30, 0, 0, paris)
	f := fois{evt: donnees.Evenement{}, debut: d}
	if q, _ := heureRappelAvant(f, rappel1h, paris); !q.Equal(d.Add(-time.Hour)) || q.Hour() != 1 || q.Minute() != 30 {
		t.Fatalf("1 h avant : %v", q)
	}
	if q, _ := heureRappelAvant(f, rappelVeille, paris); q.Day() != 27 || q.Hour() != 18 || q.Minute() != 0 {
		t.Fatalf("la veille : %v", q)
	}
	// Dimanche 31 octobre 2027 : 3 h redevient 2 h ; la veille (samedi) à 18 h.
	j := fois{evt: donnees.Evenement{Journee: 1}, debut: time.Date(2027, 11, 1, 0, 0, 0, 0, paris)}
	if q, _ := heureRappelAvant(j, rappel15min, paris); q.Day() != 31 || q.Hour() != 18 {
		t.Fatalf("journée entière : %v", q)
	}
	if _, ok := heureRappelAvant(f, "", paris); ok {
		t.Fatal("rappel sans choix")
	}
}

// Rappel juste avant : envoyé à son heure, une seule fois par fois (même
// après un redémarrage), chaque fois d'un événement qui revient, à la
// nouvelle heure d'une fois modifiée, jamais pour une fois annulée.
func TestRappelAvant(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	ctx := t.Context()
	paris, _ := time.LoadLocation("Europe/Paris")
	d0 := minuit(essai.Maintenant()).AddDate(0, 0, 3)
	cle := func(j int) string { return d0.AddDate(0, 0, j).Format("2006-01-02") }
	a10h30 := func(j int) time.Time { return time.Date(d0.Year(), d0.Month(), d0.Day()+j, 10, 30, 0, 0, paris) }

	marie.Get("/agenda/nouveau")
	// Journée entière : « 15 min avant » refusé ; la veille accepté.
	if code, _ := marie.Post("/agenda", map[string]string{"titre": "Anniversaire", "date": cle(1), "rappel_avant": "15min"}); code != http.StatusBadRequest {
		t.Fatalf("journée entière avec 15 min : %d", code)
	}
	if code, _ := marie.Post("/agenda", map[string]string{"titre": "Dentiste", "date": cle(0), "heure_debut": "10:30", "rappel_avant": "n'importe"}); code != http.StatusBadRequest {
		t.Fatalf("choix inventé accepté : %d", code)
	}
	marie.Post("/agenda", map[string]string{"titre": "Dentiste", "date": cle(0), "heure_debut": "10:30", "lieu": "Cabinet", "rappel_avant": "15min"})
	marie.Post("/agenda", map[string]string{"titre": "Anniversaire", "date": cle(1), "rappel_avant": "veille"})
	marie.Post("/agenda", map[string]string{"titre": "Piano", "date": cle(0), "heure_debut": "10:30", "repetition": "jour", "rappel_avant": "1h"})
	_, page := marie.Get("/agenda?date=" + cle(0))
	piano := essai.Trouver(t, page, `href="/agenda/(\d+)\?jour=`+cle(0)+`"`)
	dentiste := essai.Trouver(t, page, `href="/agenda/(\d+)"[^>]*>\s*<span class="heure">10:30`)
	if _, fiche := marie.Get("/agenda/" + dentiste); !strings.Contains(fiche, `<option value="15min" selected>`) {
		t.Fatal("choix du rappel non gardé")
	}

	m := &Module{a: a}
	textes := func(quand time.Time) []string {
		var l []string
		for _, e := range m.rappelsAvantDus(ctx, quand) {
			l = append(l, e.texte+" → "+e.url)
		}
		return l
	}
	// Dentiste : rien à 10:14, le rappel à 10:15, puis plus rien.
	if l := textes(a10h30(0).Add(-16 * time.Minute)); len(l) != 0 {
		t.Fatalf("trop tôt : %v", l)
	}
	l := textes(a10h30(0).Add(-15 * time.Minute))
	if len(l) != 1 || l[0] != "⏰ Dans 15 min : Dentiste à 10 h 30 (📍 Cabinet) → /agenda/"+dentiste {
		t.Fatalf("rappel du dentiste : %v", l)
	}
	if l := textes(a10h30(0).Add(-14 * time.Minute)); len(l) != 0 {
		t.Fatalf("envoyé deux fois : %v", l)
	}
	// Redémarrage du serveur (module neuf) : toujours rien (noté en base).
	m = &Module{a: a}
	if l := textes(a10h30(0).Add(-10 * time.Minute)); len(l) != 0 {
		t.Fatalf("renvoyé après un redémarrage : %v", l)
	}
	// Piano (chaque jour, 1 h avant) : chaque fois a son rappel.
	for j := 0; j < 2; j++ {
		l := textes(a10h30(j).Add(-time.Hour))
		if len(l) != 1 || !strings.HasPrefix(l[0], "⏰ Dans 1 h : Piano à 10 h 30 → /agenda/"+piano+"?jour="+cle(j)) {
			t.Fatalf("piano, fois %d : %v", j, l)
		}
	}
	// Anniversaire (journée entière) : la veille à 18 h.
	veille := time.Date(d0.Year(), d0.Month(), d0.Day(), 18, 0, 0, 0, paris)
	if l := textes(veille.Add(-time.Minute)); len(l) != 0 {
		t.Fatalf("anniversaire trop tôt : %v", l)
	}
	if l := textes(veille); len(l) != 1 || !strings.HasPrefix(l[0], "⏰ Demain : Anniversaire →") {
		t.Fatalf("anniversaire : %v", l)
	}
	// Trop tard (serveur arrêté plus de 30 min) : plus envoyé.
	if l := textes(a10h30(2).Add(-20 * time.Minute)); len(l) != 0 {
		t.Fatalf("rattrapé trop tard : %v", l)
	}

	// Fois annulée : pas de rappel ; fois modifiée : à sa nouvelle heure.
	marie.Get("/agenda/" + piano + "?jour=" + cle(3))
	marie.Post("/agenda/"+piano+"/sauter", map[string]string{"jour": cle(3)})
	if l := textes(a10h30(3).Add(-time.Hour)); len(l) != 0 {
		t.Fatalf("rappel d'une fois annulée : %v", l)
	}
	marie.Post("/agenda/"+piano+"/fois", map[string]string{"jour": cle(4), "titre": "Piano", "date": cle(4), "heure_debut": "14:00"})
	if l := textes(a10h30(4).Add(-time.Hour)); len(l) != 0 {
		t.Fatalf("rappel à l'heure d'origine d'une fois déplacée : %v", l)
	}
	quatorze := time.Date(d0.Year(), d0.Month(), d0.Day()+4, 13, 0, 0, 0, paris)
	if l := textes(quatorze); len(l) != 1 || !strings.HasPrefix(l[0], "⏰ Dans 1 h : Piano à 14 h 00 → /agenda/"+piano+"?jour="+cle(4)) {
		t.Fatalf("rappel de la fois déplacée : %v", l)
	}

	// Lien du téléphone : l'alarme suit le rappel juste avant.
	_, p := marie.Post("/agenda/calendrier", nil)
	lien := essai.Trouver(t, p, `value="(http[^"]+\.ics)"`)
	_, ics := essai.NouveauNavigateur(t, srv).Get(lien[strings.Index(lien, "/ical/"):])
	if !strings.Contains(ics, "TRIGGER:-PT15M") || !strings.Contains(ics, "TRIGGER:-PT6H") ||
		!strings.Contains(ics, "TRIGGER:-PT1H") {
		t.Fatalf("alarmes du flux :\n%s", ics)
	}

	// Agenda désactivé : aucun rappel.
	if _, err := a.DB.ExecContext(ctx, "UPDATE familles SET modules = 'courses'"); err != nil {
		t.Fatal(err)
	}
	if l := textes(a10h30(5).Add(-time.Hour)); len(l) != 0 {
		t.Fatalf("rappel avec l'agenda désactivé : %v", l)
	}
	// La tâche de chaque minute tourne sans erreur (sans service de push).
	a.TachesMinute(ctx, time.Now())
}
