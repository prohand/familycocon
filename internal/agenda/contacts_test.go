// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/contacts"
	"familycocon/internal/donnees"
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

// Revue : liste de contacts bornée avant la boucle ; carnet désactivé, les
// contacts déjà cités ne sont pas effacés par une modification.
func TestContactsBornesEtCarnetDesactive(t *testing.T) {
	a, srv := essai.NouvelleApp(t, func(a *appli.App) {
		Brancher(a)
		contacts.Brancher(a)
	})
	marie := essai.Famille(t, srv)
	marie.Get("/contacts/nouveau")
	_, page := marie.Post("/contacts", map[string]string{"nom": "Dr Dent", "telephone": "01 23 45 67 89"})
	id := essai.Trouver(t, page, `action="/contacts/(\d+)/favori"`)

	marie.Get("/agenda/nouveau")
	// 11 valeurs : refus, sans une requête par valeur.
	trop := url.Values{"titre": {"X"}, "date": {"2026-11-24"}, "_jeton": {marie.Jeton}}
	for range 11 {
		trop.Add("contacts", "9999")
	}
	rep, err := marie.C.PostForm(srv.URL+"/agenda", trop)
	if err != nil || rep.StatusCode != 400 {
		t.Fatalf("11 contacts acceptés : %v %v", err, rep.StatusCode)
	}
	rep.Body.Close()

	v := url.Values{"titre": {"Dentiste"}, "date": {"2026-11-24"}, "heure_debut": {"16:30"}, "contacts": {id, id}, "_jeton": {marie.Jeton}}
	rep, err = marie.C.PostForm(srv.URL+"/agenda", v)
	if err != nil || rep.StatusCode != 200 {
		t.Fatalf("création : %v %v", err, rep)
	}
	rep.Body.Close()
	_, page = marie.Get("/agenda?date=2026-11-24")
	evt := essai.Trouver(t, page, `href="/agenda/(\d+)"`)

	// Carnet désactivé : modifier l'événement garde le lien vers le contact.
	if _, err := a.DB.Exec(`UPDATE familles SET modules = 'agenda'`); err != nil {
		t.Fatal(err)
	}
	marie.Get("/agenda/" + evt)
	if code, _ := marie.Post("/agenda/"+evt, map[string]string{"titre": "Dentiste (déplacé)", "date": "2026-11-25", "heure_debut": "17:00"}); code != 200 {
		t.Fatalf("modification : %d", code)
	}
	var liens int
	if err := a.DB.QueryRow(`SELECT count(*) FROM evenements_contacts WHERE evenement_id = ?`, evt).Scan(&liens); err != nil || liens != 1 {
		t.Fatalf("contacts cités après modification sans carnet : %d (attendu 1) %v", liens, err)
	}
}

// Revue : un événement répété dont la répétition est finie n'est plus lu,
// et les participants lus sont ceux des événements de la période.
func TestRepetitionFinieNonChargee(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Get("/agenda/nouveau")
	marie.Post("/agenda", map[string]string{"titre": "Vieux cours", "date": "2020-01-06", "heure_debut": "10:00",
		"repetition": "semaine", "repetition_fin": "2020-06-29", "membre_1": "1"})
	marie.Post("/agenda", map[string]string{"titre": "Piscine", "date": "2026-11-02", "heure_debut": "10:00", "repetition": "semaine", "membre_1": "1"})
	paris, _ := time.LoadLocation("Europe/Paris")
	debut := time.Date(2026, 11, 23, 0, 0, 0, 0, paris)
	evts, err := a.Q.EvenementsPeriode(t.Context(), donnees.EvenementsPeriodeParams{FamilleID: 1, FinPeriode: debut.AddDate(0, 0, 7).Unix(),
		DebutPeriode: debut.Unix(), JourMin: "2025-11-22"})
	if err != nil || len(evts) != 1 || evts[0].Titre != "Piscine" {
		t.Fatalf("événements lus : %v %v", evts, err)
	}
	// La page, elle, montre bien la piscine avec sa participante, et l'ancien
	// cours reste visible à son époque.
	if _, page := marie.Get("/agenda?date=2026-11-23"); !strings.Contains(page, "Piscine") || !strings.Contains(page, "pastille c") || strings.Contains(page, "Vieux cours") {
		t.Fatal("semaine du 23 novembre 2026")
	}
	if _, page := marie.Get("/agenda?date=2020-03-02"); !strings.Contains(page, "Vieux cours") {
		t.Fatal("ancien événement répété absent de son époque")
	}
}
