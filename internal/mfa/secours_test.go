// SPDX-License-Identifier: GPL-3.0-or-later

package mfa

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestCodesSecours(t *testing.T) {
	codes := NouveauxCodesSecours()
	if len(codes) != NombreCodesSecours || len(codes[0]) != 14 || strings.Count(codes[0], "-") != 2 {
		t.Fatalf("codes : %v", codes)
	}
	// Saisie tolérante : minuscules, espaces, sans tirets.
	saisie := strings.ToLower(strings.ReplaceAll(codes[0], "-", " "))
	if !EstCodeSecours(saisie) || !bytes.Equal(EmpreinteSecours(saisie), EmpreinteSecours(codes[0])) {
		t.Fatal("saisie tolérante refusée")
	}
	if EstCodeSecours("123456") || EmpreinteSecours("0OI1-0000-0000") != nil {
		t.Fatal("faux code accepté")
	}
}

func TestTOTP(t *testing.T) {
	secret, err := NouveauSecretTOTP()
	if err != nil {
		t.Fatal(err)
	}
	maintenant := time.Unix(1_800_000_000, 0)
	code, _ := CodeTOTP(secret, maintenant)
	pas, ok := VerifierTOTP(secret, code, maintenant, 0)
	if !ok {
		t.Fatal("code courant refusé")
	}
	// Rejeu du même code : refusé.
	if _, ok := VerifierTOTP(secret, code, maintenant, pas); ok {
		t.Fatal("code rejoué accepté")
	}
	// Décalage de 30 s accepté, de 2 min refusé.
	ancien, _ := CodeTOTP(secret, maintenant.Add(-30*time.Second))
	if _, ok := VerifierTOTP(secret, ancien, maintenant, 0); !ok {
		t.Fatal("décalage de 30 s refusé")
	}
	vieux, _ := CodeTOTP(secret, maintenant.Add(-2*time.Minute))
	if _, ok := VerifierTOTP(secret, vieux, maintenant, 0); ok && vieux != code {
		t.Fatal("code de 2 min accepté")
	}
	if !strings.HasPrefix(URITOTP("marie", secret), "otpauth://totp/FamilyCocon:marie?") {
		t.Fatal(URITOTP("marie", secret))
	}
}
