// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"strings"
	"testing"
	"time"
)

func envDe(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestValeursParDefaut(t *testing.T) {
	c, err := LireDepuis(envDe(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Adresse != ":8080" || c.Inscriptions != InscriptionPremiere || c.CookieSecurise() {
		t.Fatalf("défauts inattendus : %+v", c)
	}
	if c.DureeSession != 90*24*time.Hour || c.SauvegardeNb != 7 || c.SauvegardeA != "03:30" {
		t.Fatalf("défauts inattendus : %+v", c)
	}
}

func TestLectureComplete(t *testing.T) {
	c, err := LireDepuis(envDe(map[string]string{
		"FC_URL_PUBLIQUE":     "https://famille.exemple.fr/",
		"FC_PROXYS":           "172.18.0.0/16, 10.0.0.5",
		"FC_INSCRIPTIONS":     "fermee",
		"FC_SAUVEGARDE_HEURE": "non",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.CookieSecurise() || c.URLPublique.String() != "https://famille.exemple.fr" {
		t.Fatalf("URL : %v", c.URLPublique)
	}
	if len(c.Proxys) != 2 || c.Proxys[1].Bits() != 32 {
		t.Fatalf("proxys : %v", c.Proxys)
	}
	if c.SauvegardeA != "" {
		t.Fatal("sauvegarde automatique non désactivée")
	}
	if c.ContactPush != "mailto:admin@famille.exemple.fr" {
		t.Fatal(c.ContactPush)
	}
}

func TestErreursRegroupees(t *testing.T) {
	_, err := LireDepuis(envDe(map[string]string{
		"FC_URL_PUBLIQUE": "famille.fr",
		"FC_PROXYS":       "pas-une-ip",
		"FC_INSCRIPTIONS": "peut-etre",
		"FC_NIVEAU_LOGS":  "bavard",
	}))
	if err == nil {
		t.Fatal("erreurs attendues")
	}
	for _, mot := range []string{"FC_URL_PUBLIQUE", "FC_PROXYS", "FC_INSCRIPTIONS", "FC_NIVEAU_LOGS"} {
		if !strings.Contains(err.Error(), mot) {
			t.Errorf("%s absent de : %v", mot, err)
		}
	}
}

func TestSessionMax(t *testing.T) {
	c, err := LireDepuis(envDe(nil))
	if err != nil || c.SessionMax != 365*24*time.Hour {
		t.Fatalf("défaut : %v %v", c.SessionMax, err)
	}
	// Plus court que la durée d'inactivité : refusé.
	if _, err := LireDepuis(envDe(map[string]string{"FC_DUREE_SESSION_JOURS": "90", "FC_SESSION_MAX_JOURS": "30"})); err == nil || !strings.Contains(err.Error(), "FC_SESSION_MAX_JOURS") {
		t.Fatalf("30 < 90 accepté : %v", err)
	}
}
