// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"net/url"
	"strings"
	"testing"

	"familycocon/internal/appli"
	"familycocon/internal/contacts"
	"familycocon/internal/essai"
)

// Un événement peut citer des contacts du carnet (appel direct).
func TestContactsDansAgenda(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		Brancher(a)
		contacts.Brancher(a)
	})
	marie := essai.Famille(t, srv)
	marie.Get("/contacts/nouveau")
	_, page := marie.Post("/contacts", map[string]string{"nom": "Dr Dent", "metier": "Dentiste", "pro": "1", "telephone": "01 23 45 67 89"})
	id := essai.Trouver(t, page, `action="/contacts/(\d+)/favori"`)

	_, page = marie.Get("/agenda/nouveau?date=2026-11-24")
	if !strings.Contains(page, `name="contacts" multiple`) || !strings.Contains(page, "Dr Dent (Dentiste)") {
		t.Fatal("choix des contacts absent du formulaire")
	}
	// Formulaire avec plusieurs valeurs « contacts » (dont une inconnue, ignorée).
	v := url.Values{"titre": {"Dentiste de Léo"}, "date": {"2026-11-24"}, "heure_debut": {"16:30"}, "contacts": {id, "9999"}, "_jeton": {marie.Jeton}}
	rep, err := marie.C.PostForm(srv.URL+"/agenda", v)
	if err != nil || rep.StatusCode != 200 {
		t.Fatalf("création : %v %v", err, rep)
	}
	rep.Body.Close()
	_, page = marie.Get("/agenda?date=2026-11-24")
	if !strings.Contains(page, "👤 Dr Dent") {
		t.Fatalf("contact absent de l'agenda :\n%s", page)
	}
	evt := essai.Trouver(t, page, `href="/agenda/(\d+)"`)
	if _, page := marie.Get("/agenda/" + evt); !strings.Contains(page, `href="tel:0123456789"`) {
		t.Fatal("appel direct absent de l'événement")
	}
	// Contact supprimé : l'événement reste, sans lui.
	marie.Post("/contacts/"+id+"/supprimer", nil)
	if _, page := marie.Get("/agenda?date=2026-11-24"); !strings.Contains(page, "Dentiste de Léo") || strings.Contains(page, "Dr Dent") {
		t.Fatal("après suppression du contact")
	}
}
