// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
	"familycocon/internal/suivi"
)

// Revue du 23/11/2026.
func TestRevueFoyer(t *testing.T) {
	a, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	ctx := context.Background()

	for saisie, attendu := range map[string]int64{"45000": 45000, "45 000 km": 45000, "45 000": 45000, "": 0} {
		if km, ok := lireKm(saisie); !ok || km != attendu {
			t.Errorf("lireKm(%q) = %d, %v", saisie, km, ok)
		}
	}
	for _, saisie := range []string{"abc", "-5", "6000000", "12,5"} {
		if _, ok := lireKm(saisie); ok {
			t.Errorf("lireKm(%q) accepté", saisie)
		}
	}

	// Un kilométrage illisible est refusé et ne remet pas le compteur à 0.
	marie.Get("/vehicules/nouveau")
	_, page := marie.Post("/vehicules", map[string]string{"nom": "La Clio", "genre": "voiture", "kilometrage": "45000 km"})
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)
	if code, _ := marie.Post("/vehicules/"+id, map[string]string{"nom": "La Clio", "genre": "voiture", "kilometrage": "beaucoup"}); code != http.StatusBadRequest {
		t.Fatalf("kilométrage illisible : %d", code)
	}
	if v, _ := a.Q.VehiculeParID(ctx, donnees.VehiculeParIDParams{ID: 1, FamilleID: 1}); v.Kilometrage != 45000 {
		t.Fatalf("compteur : %d", v.Kilometrage)
	}

	// Alertes : une seule requête pour la liste, la première échéance de chaque objet.
	marie.Post("/vehicules", map[string]string{"nom": "Le scooter", "genre": "moto"})
	proche := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	loin := time.Now().AddDate(0, 6, 0).Format("2006-01-02")
	marie.Post("/suivi/vehicules/"+id+"/echeances", map[string]string{"libelle": "Vidange", "jour": loin})
	marie.Post("/suivi/vehicules/"+id+"/echeances", map[string]string{"libelle": "Contrôle technique", "jour": proche})
	marie.Post("/suivi/vehicules/2/echeances", map[string]string{"libelle": "Assurance", "jour": loin})
	_, page = marie.Get("/vehicules")
	if !strings.Contains(page, "⏰ Contrôle technique : dans 5 jours") || strings.Contains(page, "⏰ Vidange") || strings.Contains(page, "⏰ Assurance") {
		t.Fatalf("alertes :\n%s", page)
	}
	// « À venir » : les noms viennent d'une seule requête.
	if _, page := marie.Get("/"); !strings.Contains(page, "Contrôle technique · La Clio") {
		t.Fatal("échéance absente de l'accueil")
	}

	// Échéance à une date absurde : refusée (elle masquait les vraies alertes).
	if code, _ := marie.Post("/suivi/vehicules/"+id+"/echeances", map[string]string{"libelle": "X", "jour": "9999-06-01"}); code != http.StatusBadRequest {
		t.Fatalf("échéance en 9999 : %d", code)
	}
	// Plafond d'échéances par objet.
	for range suivi.EcheancesMax {
		if _, err := a.Q.CreerEcheance(ctx, donnees.CreerEcheanceParams{FamilleID: 1, Module: modVehicules, ObjetID: 2, Libelle: "x", Jour: loin, CreeLe: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := marie.Post("/suivi/vehicules/2/echeances", map[string]string{"libelle": "De trop", "jour": loin}); code != http.StatusBadRequest {
		t.Fatalf("plafond d'échéances : %d", code)
	}

	// Lien e-mail d'un prestataire : adresse échappée.
	liens, _ := liensContact(donnees.Contact{Email: "a@b.fr?cc=x@y.fr", Telephone: "06 12", Site: "javascript:alert(1)"})
	for _, l := range liens {
		if strings.Contains(string(l.URL), "?cc=") || strings.HasPrefix(string(l.URL), "javascript") {
			t.Fatalf("lien dangereux : %s", l.URL)
		}
	}
}
