// SPDX-License-Identifier: GPL-3.0-or-later

// Package web contient les gabarits HTML et les fichiers statiques (CSS, JS,
// police, images), embarqués dans le binaire : un seul exécutable à déployer.
package web

import "embed"

// Fichiers : gabarits/, statique/ et ocr/ (lecture des tickets de carburant
// dans le navigateur : Tesseract.js, gardé compressé, servi à part).
//
//go:embed gabarits statique ocr
var Fichiers embed.FS
