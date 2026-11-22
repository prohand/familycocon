// SPDX-License-Identifier: GPL-3.0-or-later

// Package web contient les gabarits HTML et les fichiers statiques (CSS, JS,
// police, images), embarqués dans le binaire : un seul exécutable à déployer.
package web

import "embed"

// Fichiers : gabarits/ et statique/.
//
//go:embed gabarits statique
var Fichiers embed.FS
