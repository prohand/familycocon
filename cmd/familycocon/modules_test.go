// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"net/http/httptest"
	"testing"

	"familycocon/internal/appli"
	"familycocon/internal/essai"
)

// Un module se déclare dans appli.Catalogue et se branche dans
// brancherModules : ce test échoue si l'un des deux est oublié.
//   - module du Catalogue sans page : vérifié ici ;
//   - module branché mais absent du Catalogue : App.Route le refuse au
//     démarrage (panique), donc brancherModules échouerait ici aussi.
func TestCatalogueEtModulesBranches(t *testing.T) {
	a, _ := essai.NouvelleApp(t, func(a *appli.App) {
		if err := brancherModules(a); err != nil {
			t.Fatal(err)
		}
	})
	for _, m := range appli.Catalogue {
		_, motif := a.Mux.Handler(httptest.NewRequest("GET", m.URL, nil))
		if motif == "" {
			t.Errorf("module %q du Catalogue : aucune page branchée pour %s (cmd/familycocon/modules.go)", m.Cle, m.URL)
		}
	}
}

// Une clé de module inconnue est refusée dès le démarrage.
func TestModuleInconnuRefuse(t *testing.T) {
	a, _ := essai.NouvelleApp(t, nil)
	defer func() {
		if recover() == nil {
			t.Fatal("route d'un module absent du Catalogue acceptée")
		}
	}()
	a.Route("GET /inconnu", "inconnu", appli.Membre, nil)
}
