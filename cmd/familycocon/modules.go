// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"familycocon/internal/agenda"
	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/courses"
	"familycocon/internal/enfants"
	"familycocon/internal/menus"
	"familycocon/internal/taches"
)

// brancherModules ajoute les routes de chaque module (courses, agenda…).
func brancherModules(a *appli.App) error {
	courses.Brancher(a)
	taches.Brancher(a)
	agenda.Brancher(a)
	menus.Brancher(a)
	budget.Brancher(a)
	enfants.Brancher(a)
	return nil
}
