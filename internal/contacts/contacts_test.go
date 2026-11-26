// SPDX-License-Identifier: GPL-3.0-or-later

package contacts

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/essai"
)

func TestNaissance(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Date(2026, 11, 22, 0, 0, 0, 0, paris)
	n, ok := LireNaissance("2018-03-05")
	if !ok || n.Prochain(auj) != time.Date(2027, 3, 5, 0, 0, 0, 0, paris) || n.Age(n.Prochain(auj)) != 9 {
		t.Fatal("anniversaire de 2018")
	}
	// Aujourd'hui : c'est aujourd'hui, pas l'an prochain.
	if n, _ := LireNaissance("1990-11-22"); !n.Prochain(auj).Equal(auj) {
		t.Fatal("anniversaire du jour")
	}
	// 29 février : fêté le 28 en 2027.
	if n, _ := LireNaissance("2016-02-29"); n.Prochain(auj).Day() != 28 {
		t.Fatal("29 février")
	}
	// Année inconnue : pas d'âge, 29 février accepté.
	if n, ok := LireNaissance("0000-02-29"); !ok || n.Age(auj) != 0 {
		t.Fatal("année inconnue")
	}
	for _, faux := range []string{"", "2018-02-30", "2018-13-01", "18-03-05"} {
		if _, ok := LireNaissance(faux); ok {
			t.Errorf("%q accepté", faux)
		}
	}
}

func TestAnniversairesEtAdresses(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	paris, _ := time.LoadLocation("Europe/Paris")
	dans3 := time.Now().In(paris).AddDate(0, 0, 3)

	// Anniversaire de Mamie dans 3 jours (née en 1946), sans téléphone.
	marie.Get("/contacts/nouveau?anniv=1")
	code, page := marie.Post("/contacts", map[string]string{"anniv": "1", "nom": "Mamie Jeanne", "relation": "famille",
		"naissance": dans3.AddDate(-80, 0, 0).Format("2006-01-02"), "idees": "un plaid"})
	if code != 200 || !strings.Contains(page, "Mamie Jeanne") || !strings.Contains(page, "80 ans") || !strings.Contains(page, "Dans 3 jours") {
		t.Fatalf("anniversaire : %d\n%s", code, page)
	}
	// Ami, année inconnue, avec coordonnées.
	marie.Get("/contacts/nouveau")
	code, page = marie.Post("/contacts", map[string]string{"nom": "Jules (ami de Hugo)", "relation": "ami",
		"naissance": "2000-07-14", "sans_annee": "1", "telephone": "06 12 34 56 78", "email": "parents.jules@example.org", "adresse": "3 rue des Lilas\n75000 Paris"})
	if code != 200 || !strings.Contains(page, `href="tel:0612345678"`) || !strings.Contains(page, "openstreetmap.org/search?query=3%20rue%20des%20Lilas") || !strings.Contains(page, "14 juillet") {
		t.Fatalf("fiche : %d\n%s", code, page)
	}
	if code, _ := marie.Post("/contacts", map[string]string{"nom": "X", "email": "pas-un-mail"}); code != 400 {
		t.Fatalf("e-mail invalide accepté : %d", code)
	}
	// Accueil : « À venir ».
	if _, page := marie.Get("/"); !strings.Contains(page, "À venir") || !strings.Contains(page, "Anniversaire de Mamie Jeanne (80 ans)") {
		t.Fatal("bloc À venir absent")
	}
	// Recherche dans le carnet.
	if _, page := marie.Get("/adresses?q=lilas"); !strings.Contains(page, "Jules") || strings.Contains(page, "Mamie") {
		t.Fatal("recherche dans le carnet")
	}
	// Date de naissance d'un membre (page Moi) : apparaît dans les anniversaires.
	marie.Get("/moi")
	marie.Post("/moi/profil", map[string]string{"prenom": "Marie", "couleur": "1", "naissance": "1988-01-15"})
	if _, page := marie.Get("/anniversaires"); !strings.Contains(page, "Marie") {
		t.Fatal("anniversaire d'un membre absent")
	}
	// Un enfant consulte et corrige, mais ne supprime pas.
	_, page = lea.Get("/adresses")
	id := essai.Trouver(t, page, `href="/contacts/(\d+)"`)
	if code, _ := lea.Post("/contacts/"+id+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression par un enfant : %d", code)
	}
	// Modules désactivés : fiches inaccessibles.
	marie.Get("/famille")
	marie.Post("/famille/modules", map[string]string{"module_courses": "1"})
	if code, _ := marie.Get("/contacts/" + id); code != http.StatusNotFound {
		t.Fatalf("fiche accessible modules désactivés : %d", code)
	}
}

// Anniversaire proche d'un contact qui a un portable : appeler et SMS déjà
// écrit ; sans numéro, rien ; loin, rien.
func TestAnniversaireSouhaiter(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	auj := essai.Maintenant()
	marie.Get("/contacts/nouveau")
	marie.Post("/contacts", map[string]string{"nom": "Paul Martin", "naissance": auj.AddDate(-30, 0, 2).Format("2006-01-02"), "telephone": "06 12 34 56 78"})
	marie.Post("/contacts", map[string]string{"nom": "Mamie", "naissance": auj.AddDate(-80, 0, 1).Format("2006-01-02")})
	marie.Post("/contacts", map[string]string{"nom": "Loin", "naissance": auj.AddDate(-20, 3, 0).Format("2006-01-02"), "telephone": "0699999999"})
	_, page := marie.Get("/anniversaires")
	if !strings.Contains(page, `href="sms:0612345678?&amp;body=Joyeux%20anniversaire%20Paul%20%21%20%F0%9F%8E%82"`) || !strings.Contains(page, `href="tel:0612345678"`) {
		t.Fatalf("boutons de Paul absents :\n%s", page)
	}
	if strings.Contains(page, "0699999999") {
		t.Fatal("boutons montrés pour un anniversaire lointain")
	}
	if n := strings.Count(page, `class="appel"`); n != 2 {
		t.Fatalf("%d boutons, attendu 2 (Paul seulement)", n)
	}
}
