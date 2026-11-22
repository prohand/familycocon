// SPDX-License-Identifier: GPL-3.0-or-later

// Repris de mirador-stack (internal/server/auth/secours.go), même auteur ;
// la partie base de données est dans internal/donnees (sqlc).

package mfa

import (
	"crypto/rand"
	"crypto/sha256"
	"strings"
)

// Codes de secours de la double authentification : si le téléphone est
// perdu, un code remplace le code de l'application, une seule fois. Dix
// codes par compte, affichés une seule fois à leur création ; en créer de
// nouveaux annule les anciens. En base : seulement leur empreinte.

// NombreCodesSecours : codes créés à chaque fois.
const NombreCodesSecours = 10

// Alphabet sans les caractères les plus ambigus (ni 0 ni O, ni 1 ni I) :
// 32 signes, soit 5 bits par signe.
const alphabetSecours = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NouveauxCodesSecours : dix codes de 12 signes (60 bits d'aléa), écrits
// « XXXX-XXXX-XXXX ».
func NouveauxCodesSecours() []string {
	codes := make([]string, 0, NombreCodesSecours)
	for len(codes) < NombreCodesSecours {
		b := make([]byte, 12)
		_, _ = rand.Read(b) // ne renvoie jamais d'erreur (Go ≥ 1.24)
		var s strings.Builder
		for i, o := range b {
			if i > 0 && i%4 == 0 {
				s.WriteByte('-')
			}
			s.WriteByte(alphabetSecours[o&31])
		}
		codes = append(codes, s.String())
	}
	return codes
}

// normaliserSecours : majuscules, sans espaces ni tirets ; "" si la forme
// n'est pas celle d'un code de secours.
func normaliserSecours(code string) string {
	var s strings.Builder
	for _, r := range strings.ToUpper(code) {
		switch {
		case r == ' ' || r == '-':
		case strings.ContainsRune(alphabetSecours, r):
			s.WriteRune(r)
		default:
			return ""
		}
	}
	if s.Len() != 12 {
		return ""
	}
	return s.String()
}

// EmpreinteSecours : empreinte gardée en base (nil si ce n'est pas un code
// de secours bien formé).
func EmpreinteSecours(code string) []byte {
	n := normaliserSecours(code)
	if n == "" {
		return nil
	}
	h := sha256.Sum256([]byte("familycocon-secours:" + n))
	return h[:]
}

// EstCodeSecours dit si le texte a la forme d'un code de secours (et pas
// celle d'un code à 6 chiffres).
func EstCodeSecours(code string) bool { return normaliserSecours(code) != "" }
