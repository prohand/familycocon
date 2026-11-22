// SPDX-License-Identifier: GPL-3.0-or-later

// Repris de mirador-stack (internal/server/auth), même auteur.

package mfa

import (
	"errors"
	"strings"
	"testing"

	"familycocon/internal/mfa/fauxcle"
)

// dbTest : base du test en cours (posée par baseTest).

const (
	domaineTest = "famille.example.org"
	origineTest = "https://famille.example.org"
)

// Création puis connexion, pour les trois algorithmes acceptés.
func TestPasskeyCreationEtConnexion(t *testing.T) {
	for nom, alg := range map[string]int{"ES256": algES256, "EdDSA": algEdDSA, "RS256": algRS256} {
		t.Run(nom, func(t *testing.T) {
			cle := fauxcle.Nouvelle(alg, domaineTest, origineTest)
			defi, _ := NouveauDefiPasskey()
			client, att := cle.Creer(defi)
			p, err := VerifierCreation(client, att, defi, domaineTest, origineTest)
			if err != nil {
				t.Fatal(err)
			}
			if p.Algorithme != alg || string(p.Identifiant) != string(cle.Identifiant) || p.Domaine != domaineTest {
				t.Fatalf("clé enregistrée : %+v", p)
			}
			defi2, _ := NouveauDefiPasskey()
			c, a, s := cle.Signer(defi2)
			if _, err := VerifierConnexion(p, c, a, s, defi2, domaineTest, origineTest, false); err != nil {
				t.Fatalf("connexion refusée : %v", err)
			}
			// Tout ce qui doit être refusé à la connexion.
			autre := fauxcle.Nouvelle(alg, domaineTest, origineTest)
			_, _, sigAutre := autre.Signer(defi2)
			ailleurs := *cle
			ailleurs.Origine = "https://famille.example.org.pirate.example"
			cA, aA, sA := ailleurs.Signer(defi2)
			refuses := map[string][3]string{
				"défi rejoué (un autre défi est attendu)": {c, a, s},
				"signature d'une autre clé":               {c, a, sigAutre},
				"origine d'hameçonnage":                   {cA, aA, sA},
				"signature tronquée":                      {c, a, s[:len(s)-4]},
			}
			for cas, r := range refuses {
				attendu := defi2
				if strings.HasPrefix(cas, "défi") {
					attendu, _ = NouveauDefiPasskey()
				}
				if _, err := VerifierConnexion(p, r[0], r[1], r[2], attendu, domaineTest, origineTest, false); !errors.Is(err, ErrPasskey) {
					t.Errorf("%s : accepté (%v)", cas, err)
				}
			}
			// Clé créée pour un autre domaine : inutilisable ici.
			if _, err := VerifierConnexion(p, c, a, s, defi2, "autre.example.org", "https://autre.example.org", false); !errors.Is(err, ErrPasskey) {
				t.Error("clé acceptée sous un autre domaine")
			}
		})
	}
}

func TestPasskeyCreationRefusee(t *testing.T) {
	cle := fauxcle.Nouvelle(algES256, domaineTest, origineTest)
	defi, _ := NouveauDefiPasskey()
	client, att := cle.Creer(defi)
	autreDefi, _ := NouveauDefiPasskey()
	// Une réponse de connexion ne vaut pas création (type webauthn.get).
	clientGet, _, _ := cle.Signer(defi)
	cas := map[string][4]string{
		"défi faux":          {client, att, autreDefi, origineTest},
		"défi vide":          {client, att, "", origineTest},
		"autre origine":      {client, att, defi, "https://pirate.example"},
		"mauvais type":       {clientGet, att, defi, origineTest},
		"attestation abîmée": {client, att[:len(att)/2], defi, origineTest},
		"attestation vide":   {client, "", defi, origineTest},
		"pas du base64":      {"***", att, defi, origineTest},
		"attestation énorme": {client, strings.Repeat("A", 200_000), defi, origineTest},
	}
	for nom, c := range cas {
		if _, err := VerifierCreation(c[0], c[1], c[2], domaineTest, c[3]); err == nil {
			t.Errorf("%s : accepté", nom)
		}
	}
	if _, err := VerifierCreation(client, att, defi, "autre.example.org", origineTest); !errors.Is(err, ErrPasskey) {
		t.Error("clé acceptée pour un autre domaine")
	}
}

// Passkey seule (sans mot de passe) : l'appareil doit avoir vérifié
// l'utilisateur ; une clé qui ne fait que « toucher » est refusée.
func TestPasskeyVerificationExigee(t *testing.T) {
	cle := fauxcle.Nouvelle(algES256, domaineTest, origineTest)
	defi, _ := NouveauDefiPasskey()
	client, att := cle.Creer(defi)
	p, _ := VerifierCreation(client, att, defi, domaineTest, origineTest)
	c, a, s := cle.Signer(defi)
	if _, err := VerifierConnexion(p, c, a, s, defi, domaineTest, origineTest, true); err != nil {
		t.Fatalf("clé avec vérification refusée : %v", err)
	}
	cle.SansVerification = true
	c, a, s = cle.Signer(defi)
	if _, err := VerifierConnexion(p, c, a, s, defi, domaineTest, origineTest, true); !errors.Is(err, ErrPasskey) {
		t.Error("clé sans vérification de l'utilisateur acceptée seule")
	}
	if _, err := VerifierConnexion(p, c, a, s, defi, domaineTest, origineTest, false); err != nil {
		t.Errorf("la même clé doit rester valable après un mot de passe : %v", err)
	}
}

// Compteur de signatures : il doit avancer quand la clé s'en sert.
func TestPasskeyCompteur(t *testing.T) {
	cle := fauxcle.Nouvelle(algES256, domaineTest, origineTest)
	cle.Compteur = 5
	defi, _ := NouveauDefiPasskey()
	client, att := cle.Creer(defi)
	p, err := VerifierCreation(client, att, defi, domaineTest, origineTest)
	if err != nil || p.Compteur != 5 {
		t.Fatalf("%+v %v", p, err)
	}
	c, a, s := cle.Signer(defi)
	n, err := VerifierConnexion(p, c, a, s, defi, domaineTest, origineTest, false)
	if err != nil || n != 6 {
		t.Fatalf("compteur %d %v", n, err)
	}
	p.Compteur = 9 // la base a vu plus loin : la réponse vient d'une copie
	c, a, s = cle.Signer(defi)
	if _, err := VerifierConnexion(p, c, a, s, defi, domaineTest, origineTest, false); !errors.Is(err, ErrPasskey) {
		t.Error("compteur en recul accepté")
	}
}

func TestDomaineEtOrigine(t *testing.T) {
	for hote, attendu := range map[string][2]string{
		"famille.example.org":     {"famille.example.org", "https://famille.example.org"},
		"Famille.Example.org:444": {"famille.example.org", "https://Famille.Example.org:444"},
		"localhost:9441":          {"localhost", "http://localhost:9441"},
	} {
		d, o, err := DomaineEtOrigine(hote)
		if err != nil || d != attendu[0] || o != attendu[1] {
			t.Errorf("%s : %s %s %v", hote, d, o, err)
		}
	}
	for _, mauvais := range []string{"", "a/b", "a b", "moi@pirate.example"} {
		if _, _, err := DomaineEtOrigine(mauvais); err == nil {
			t.Errorf("%q accepté", mauvais)
		}
	}
	if PasskeyPossible("192.0.2.1") || PasskeyPossible("::1") || !PasskeyPossible("famille.example.org") {
		t.Error("PasskeyPossible : une adresse IP n'est pas un domaine")
	}
}
