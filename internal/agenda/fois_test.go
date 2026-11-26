// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/essai"
)

// jourDe : la partie de la vue semaine d'un jour (de son titre au suivant).
func jourDe(t *testing.T, page, titre string) string {
	t.Helper()
	i := strings.Index(page, "<h3>"+titre)
	if i < 0 {
		t.Fatalf("jour %q absent de la page", titre)
	}
	reste := page[i+4:]
	if j := strings.Index(reste, "<h3>"); j >= 0 {
		reste = reste[:j]
	}
	return reste
}

// « Modifier cette fois seulement » : la fois quitte sa date d'origine et
// apparaît à sa nouvelle date (agenda, fiche, « À venir », accueil, lien du
// téléphone) ; les autres fois ne changent pas ; « Comme les autres fois »
// la remet ; annuler une fois modifiée l'annule.
func TestModifierUneFois(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	// Chaque semaine, à partir de demain, 17:00-18:00, au gymnase, à rappeler.
	maintenant := essai.Maintenant()
	d0 := minuit(maintenant).AddDate(0, 0, 1)
	cle := func(j int) string { return d0.AddDate(0, 0, j).Format("2006-01-02") }
	// Le contenu du jour d0+j dans la vue semaine.
	jour := func(j int) string {
		t.Helper()
		_, page := marie.Get("/agenda?date=" + cle(j))
		return jourDe(t, page, appli.TitreJour(d0.AddDate(0, 0, j)))
	}
	marie.Get("/agenda/nouveau")
	marie.Post("/agenda", map[string]string{"titre": "Judo", "date": cle(0), "heure_debut": "17:00", "heure_fin": "18:00",
		"lieu": "Gymnase", "repetition": "semaine", "rappel": "1"})
	_, page := marie.Get("/agenda?date=" + cle(7))
	id := essai.Trouver(t, page, `href="/agenda/(\d+)\?jour=`+cle(7)+`"`)

	// La fiche ouverte depuis la 2e fois propose de la modifier seule.
	_, fiche := marie.Get("/agenda/" + id + "?jour=" + cle(7))
	if !strings.Contains(fiche, "Modifier cette fois seulement") || !strings.Contains(fiche, `action="/agenda/`+id+`/fois"`) {
		t.Fatalf("formulaire « cette fois » absent :\n%s", fiche)
	}
	// Refus : date qui n'est pas une fois, enfant qui ne l'a pas créé.
	if code, _ := marie.Post("/agenda/"+id+"/fois", map[string]string{"jour": cle(8), "titre": "Judo", "date": cle(8)}); code != http.StatusBadRequest {
		t.Fatalf("jour sans événement accepté : %d", code)
	}
	lea.Get("/agenda")
	if code, _ := lea.Post("/agenda/"+id+"/fois", map[string]string{"jour": cle(7), "titre": "Judo", "date": cle(8)}); code != http.StatusForbidden {
		t.Fatalf("modifié par un enfant : %d", code)
	}
	if code, _ := marie.Post("/agenda/"+id+"/fois", map[string]string{"jour": cle(7), "titre": "Judo", "date": cle(7),
		"heure_debut": "18:00", "heure_fin": "17:00"}); code != http.StatusBadRequest {
		t.Fatalf("fin avant le début acceptée : %d", code)
	}

	// La 2e fois passe au lendemain, 18:00-19:30, à la piscine, autre titre.
	marie.Get("/agenda/" + id + "?jour=" + cle(7))
	if code, _ := marie.Post("/agenda/"+id+"/fois", map[string]string{"jour": cle(7), "titre": "Judo rattrapage", "date": cle(8),
		"heure_debut": "18:00", "heure_fin": "19:30", "lieu": "Piscine"}); code != 200 {
		t.Fatalf("modification : %d", code)
	}
	origine, nouveau := jour(7), jour(8)
	if strings.Contains(origine, "Judo") {
		t.Fatalf("la fois est encore à sa date d'origine :\n%s", origine)
	}
	if !strings.Contains(nouveau, "Judo rattrapage") || !strings.Contains(nouveau, "Piscine") || !strings.Contains(nouveau, "18:00") ||
		!strings.Contains(nouveau, "modifié cette fois") || !strings.Contains(nouveau, `href="/agenda/`+id+`?jour=`+cle(7)+`"`) {
		t.Fatalf("la fois modifiée n'est pas à sa nouvelle date :\n%s", nouveau)
	}
	// Les autres fois ne changent pas.
	if j := jour(14); !strings.Contains(j, "Judo") || !strings.Contains(j, "Gymnase") || strings.Contains(j, "rattrapage") {
		t.Fatalf("une autre fois a changé :\n%s", j)
	}
	// La fiche dit que cette fois est modifiée, et la liste des fois aussi.
	_, fiche = marie.Get("/agenda/" + id + "?jour=" + cle(7))
	if !strings.Contains(fiche, "est modifiée") || !strings.Contains(fiche, `value="Judo rattrapage"`) || !strings.Contains(fiche, "Comme les autres fois") {
		t.Fatalf("fiche de la fois modifiée :\n%s", fiche)
	}
	if _, fiche = marie.Get("/agenda/" + id); !strings.Contains(fiche, "Fois annulées ou modifiées") || !strings.Contains(fiche, "18 h 00") {
		t.Fatalf("liste des fois modifiées :\n%s", fiche)
	}
	// « À venir » (rappel du matin) : à la nouvelle date, avec le nouveau titre.
	if _, accueil := marie.Get("/"); !strings.Contains(accueil, "Judo rattrapage à 18 h 00") {
		t.Fatalf("« À venir » sans la fois modifiée :\n%s", accueil)
	}

	// Lien du téléphone : même UID (date d'origine), nouvelle heure.
	_, p := marie.Post("/agenda/calendrier", nil)
	lien := essai.Trouver(t, p, `value="(http[^"]+\.ics)"`)
	_, ics := essai.NouveauNavigateur(t, srv).Get(lien[strings.Index(lien, "/ical/"):])
	uid := "UID:" + id + "-" + strings.ReplaceAll(cle(7), "-", "") + "@familycocon"
	i := strings.Index(ics, uid)
	if i < 0 {
		t.Fatalf("fois modifiée absente du flux :\n%s", ics)
	}
	vevent := ics[i:]
	vevent = vevent[:strings.Index(vevent, "END:VEVENT")]
	nouvelle := time.Date(d0.Year(), d0.Month(), d0.Day()+8, 18, 0, 0, 0, d0.Location())
	if !strings.Contains(vevent, "DTSTART:"+nouvelle.UTC().Format("20060102T150405Z")) || !strings.Contains(vevent, "SUMMARY:Judo rattrapage") ||
		!strings.Contains(vevent, "LOCATION:Piscine") {
		t.Fatalf("fois modifiée dans le flux :\n%s", vevent)
	}
	if strings.Count(ics, uid) != 1 {
		t.Fatal("fois en double dans le flux")
	}

	// Déplacée à aujourd'hui : dans « Aujourd'hui » de l'accueil.
	marie.Get("/agenda/" + id + "?jour=" + cle(14))
	marie.Post("/agenda/"+id+"/fois", map[string]string{"jour": cle(14), "titre": "Judo exceptionnel", "date": maintenant.Format("2006-01-02")})
	if _, accueil := marie.Get("/"); !strings.Contains(accueil, "Judo exceptionnel") {
		t.Fatalf("fois déplacée à aujourd'hui absente de l'accueil :\n%s", accueil)
	}

	// « Comme les autres fois » : la 2e fois revient à sa date.
	marie.Post("/agenda/"+id+"/retablir", map[string]string{"jour": cle(7)})
	if j := jour(7); !strings.Contains(j, "Gymnase") || strings.Contains(jour(8), "rattrapage") {
		t.Fatal("fois non rétablie")
	}
	// Mêmes valeurs que les autres fois : rien n'est gardé.
	marie.Post("/agenda/"+id+"/fois", map[string]string{"jour": cle(7), "titre": "Judo", "date": cle(7),
		"heure_debut": "17:00", "heure_fin": "18:00", "lieu": "Gymnase"})
	if _, fiche = marie.Get("/agenda/" + id + "?jour=" + cle(7)); strings.Contains(fiche, "est modifiée") {
		t.Fatal("fois identique gardée comme modifiée")
	}
	// Annuler une fois modifiée : elle n'a plus lieu nulle part.
	marie.Post("/agenda/"+id+"/fois", map[string]string{"jour": cle(7), "titre": "Judo", "date": cle(8), "heure_debut": "18:00"})
	marie.Post("/agenda/"+id+"/sauter", map[string]string{"jour": cle(7)})
	if strings.Contains(jour(7), "Judo") || strings.Contains(jour(8), "Judo") {
		t.Fatal("fois modifiée puis annulée encore affichée")
	}
}
