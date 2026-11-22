// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"familycocon/internal/appli"
	"familycocon/internal/courses"
)

// brancherModules ajoute les routes de chaque module (courses, agenda…).
func brancherModules(a *appli.App) error {
	courses.Brancher(a)
	return nil
}
