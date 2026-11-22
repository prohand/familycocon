// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"familycocon/internal/abonnements"
	"familycocon/internal/agenda"
	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/contacts"
	"familycocon/internal/courses"
	"familycocon/internal/enfants"
	"familycocon/internal/fidelite"
	"familycocon/internal/foyer"
	"familycocon/internal/menus"
	"familycocon/internal/notes"
	"familycocon/internal/projets"
	"familycocon/internal/suivi"
	"familycocon/internal/taches"
)

// brancherModules ajoute les routes de chaque module (courses, agenda…).
func brancherModules(a *appli.App) error {
	courses.Brancher(a)
	taches.Brancher(a)
	agenda.Brancher(a)
	menus.Brancher(a)
	budget.Brancher(a)
	abonnements.Brancher(a)
	enfants.Brancher(a)
	notes.Brancher(a)
	contacts.Brancher(a)
	fidelite.Brancher(a)
	s := suivi.Brancher(a) // échéances et carnet communs
	foyer.Brancher(a, s)
	projets.Brancher(a, s)
	return nil
}
