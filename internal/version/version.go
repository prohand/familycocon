// SPDX-License-Identifier: GPL-3.0-or-later

// Package version : version et commit du binaire, fixés à la compilation
// (-ldflags "-X familycocon/internal/version.Version=…").
package version

var (
	// Version publiée (ex. 0.1.0), « dev » pour une compilation locale.
	Version = "dev"
	// Commit Git d'où vient le binaire (vide en local).
	Commit = ""
)
