// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"familycocon/internal/appli"
	"familycocon/internal/courses"
	"familycocon/internal/taches"
)

// brancherModules ajoute les routes de chaque module (courses, agenda…).
func brancherModules(a *appli.App) error {
	courses.Brancher(a)
	taches.Brancher(a)
	return nil
}
