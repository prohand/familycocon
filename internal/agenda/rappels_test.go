// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"context"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

// Un événement « à rappeler » apparaît dans « À venir » ; un événement sans
// rappel, non. Les échéances des autres modules sont visibles dans l'agenda.
func TestRappelEvenementEtEcheancesDansAgenda(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		Brancher(a)
		// Un module qui fournit une échéance (comme le contrôle technique).
		a.AjouterEcheances("vehicules", func(_ context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
			j := debut.AddDate(0, 0, 2)
			if !j.Before(fin) {
				return nil, nil
			}
			return []appli.Echeance{{Jour: j, Titre: "Contrôle technique", Emoji: "🚗", URL: "/vehicules/1", Module: "vehicules"}}, nil
		})
	})
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	dans3 := time.Now().In(paris).AddDate(0, 0, 3).Format("2006-01-02")
	marie.Get("/agenda/nouveau")
	marie.Post("/agenda", map[string]string{"titre": "Dentiste", "date": dans3, "heure_debut": "10:30", "rappel": "1"})
	marie.Post("/agenda", map[string]string{"titre": "Piscine", "date": dans3, "heure_debut": "17:00"})
	_, page := marie.Get("/")
	if !strings.Contains(page, "Dentiste à 10 h 30") {
		t.Fatalf("événement à rappeler absent de « À venir » :\n%s", page)
	}
	if strings.Contains(page, "Piscine") {
		t.Fatal("événement sans rappel dans « À venir »")
	}
	// La case reste cochée à la modification.
	_, page = marie.Get("/agenda?date=" + dans3)
	id := essai.Trouver(t, page, `href="/agenda/(\d+)[?"][^>]*>\s*<span class="heure">10:30`)
	if _, fiche := marie.Get("/agenda/" + id); !strings.Contains(fiche, `name="rappel" value="1" checked`) {
		t.Fatal("case « Me le rappeler » non cochée")
	}
	// L'échéance d'un autre module est dans l'agenda.
	if !strings.Contains(page, "🚗 Contrôle technique") {
		t.Fatalf("échéance absente de l'agenda :\n%s", page)
	}
}

// « Annuler cette fois seulement » : la fois disparaît, les autres restent ;
// « Rétablir » la remet.
func TestAnnulerUneFois(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Get("/agenda/nouveau")
	marie.Post("/agenda", map[string]string{"titre": "Judo", "date": "2026-11-17", "heure_debut": "17:00", "repetition": "semaine"})
	_, page := marie.Get("/agenda?date=2026-11-24")
	id := essai.Trouver(t, page, `href="/agenda/(\d+)\?jour=2026-11-24"`)
	if _, fiche := marie.Get("/agenda/" + id + "?jour=2026-11-24"); !strings.Contains(fiche, "Annuler cette fois seulement") {
		t.Fatal("bouton absent")
	}
	if code, _ := marie.Post("/agenda/"+id+"/sauter", map[string]string{"jour": "2026-11-25"}); code != 400 {
		t.Fatalf("jour sans événement accepté : %d", code)
	}
	marie.Post("/agenda/"+id+"/sauter", map[string]string{"jour": "2026-11-24"})
	if _, page = marie.Get("/agenda?date=2026-11-24"); strings.Contains(page, "Judo") {
		t.Fatal("fois annulée toujours affichée")
	}
	if _, page = marie.Get("/agenda?date=2026-12-01"); !strings.Contains(page, "Judo") {
		t.Fatal("les autres fois ont disparu")
	}
	marie.Post("/agenda/"+id+"/retablir", map[string]string{"jour": "2026-11-24"})
	if _, page = marie.Get("/agenda?date=2026-11-24"); !strings.Contains(page, "Judo") {
		t.Fatal("fois rétablie absente")
	}
}

// Lien secret iCal : flux lisible sans session, fois annulées absentes,
// texte échappé ; un nouveau lien rend l'ancien inutile ; coupé, plus rien.
func TestFluxICal(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	dans2 := time.Now().In(paris).AddDate(0, 0, 2).Format("2006-01-02")
	marie.Get("/agenda/nouveau")
	marie.Post("/agenda", map[string]string{"titre": "Dentiste, contrôle; Léo", "date": dans2, "heure_debut": "10:00", "heure_fin": "10:30", "rappel": "1"})
	marie.Post("/agenda", map[string]string{"titre": "Anniversaire", "date": dans2})
	_, page := marie.Post("/agenda/calendrier", nil)
	lien := essai.Trouver(t, page, `value="(http[^"]+\.ics)"`)
	chemin := lien[strings.Index(lien, "/ical/"):]
	anonyme := essai.NouveauNavigateur(t, srv)
	code, ics := anonyme.Get(chemin)
	if code != 200 || !strings.Contains(ics, "BEGIN:VCALENDAR") || !strings.Contains(ics, `SUMMARY:Dentiste\, contrôle\; Léo`) ||
		!strings.Contains(ics, "TRIGGER:-PT1H") || !strings.Contains(ics, "DTSTART;VALUE=DATE:") || !strings.Contains(ics, "\r\n") {
		t.Fatalf("flux : %d\n%s", code, ics)
	}
	if _, page = marie.Get("/agenda/calendrier"); !strings.Contains(page, "Dernière lecture") {
		t.Fatal("lecture non notée")
	}
	// Nouveau lien : l'ancien ne marche plus.
	marie.Post("/agenda/calendrier", nil)
	if code, _ := anonyme.Get(chemin); code != 404 {
		t.Fatalf("ancien lien encore valable : %d", code)
	}
	if code, _ := anonyme.Get("/ical/nimportequoi.ics"); code != 404 {
		t.Fatal("jeton inventé accepté")
	}
}
