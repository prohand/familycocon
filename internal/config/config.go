// SPDX-License-Identifier: GPL-3.0-or-later

// Package config lit la configuration dans les variables d'environnement
// (fichier .env chargé par Docker Compose). Tout a une valeur par défaut
// raisonnable : seule FC_URL_PUBLIQUE est à régler en production.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Inscriptions : qui peut créer une nouvelle famille.
const (
	InscriptionPremiere = "premiere" // seulement tant qu'aucune famille n'existe (défaut)
	InscriptionOuverte  = "ouverte"  // tout le monde (serveur partagé entre plusieurs familles)
	InscriptionFermee   = "fermee"   // personne
)

// Config : réglages du serveur.
type Config struct {
	Adresse      string         // adresse d'écoute, ex. « :8080 »
	Donnees      string         // dossier des données (base, sauvegardes, clés)
	URLPublique  *url.URL       // adresse vue par les navigateurs (liens d'invitation, cookies Secure)
	Proxys       []netip.Prefix // reverse proxys de confiance (X-Forwarded-For lu seulement venant d'eux)
	Inscriptions string         // premiere | ouverte | fermee
	NiveauLogs   slog.Level
	FormatLogs   string        // json | texte
	SauvegardeA  string        // heure de la sauvegarde automatique (« 03:30 »), vide = aucune
	SauvegardeNb int           // nombre de sauvegardes automatiques gardées
	DureeSession time.Duration // sans visite pendant ce temps, la session expire
	SessionMax   time.Duration // durée maximale d'une session, même utilisée chaque jour
	ContactPush  string        // adresse « mailto: » ou « https: » transmise aux services de push (VAPID)
}

// CookieSecurise : cookies « Secure » dès que l'adresse publique est en https.
func (c *Config) CookieSecurise() bool { return c.URLPublique.Scheme == "https" }

// Lire construit la configuration depuis l'environnement.
func Lire() (*Config, error) {
	return LireDepuis(os.Getenv)
}

// LireDepuis : comme Lire, avec une fonction de lecture (pour les tests).
func LireDepuis(env func(string) string) (*Config, error) {
	val := func(nom, defaut string) string {
		if v := strings.TrimSpace(env(nom)); v != "" {
			return v
		}
		return defaut
	}
	var errs []error
	c := &Config{
		Adresse:      val("FC_ADRESSE", ":8080"),
		Donnees:      val("FC_DONNEES", "./donnees"),
		Inscriptions: val("FC_INSCRIPTIONS", InscriptionPremiere),
		FormatLogs:   val("FC_FORMAT_LOGS", "texte"),
		SauvegardeA:  val("FC_SAUVEGARDE_HEURE", "03:30"),
		ContactPush:  val("FC_CONTACT_PUSH", ""),
	}

	u, err := url.Parse(val("FC_URL_PUBLIQUE", "http://localhost:8080"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, errors.New("FC_URL_PUBLIQUE : adresse http(s) complète attendue, ex. https://famille.exemple.fr"))
		u = &url.URL{Scheme: "http", Host: "localhost:8080"}
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	c.URLPublique = u

	for _, p := range strings.FieldsFunc(env("FC_PROXYS"), func(r rune) bool { return r == ',' || r == ' ' }) {
		pre, err := netip.ParsePrefix(p)
		if err != nil {
			// Une adresse seule est acceptée (ex. 172.18.0.2).
			a, err2 := netip.ParseAddr(p)
			if err2 != nil {
				errs = append(errs, fmt.Errorf("FC_PROXYS : %q n'est ni une adresse ni un réseau (ex. 172.18.0.0/16)", p))
				continue
			}
			pre = netip.PrefixFrom(a, a.BitLen())
		}
		c.Proxys = append(c.Proxys, pre.Masked())
	}

	switch c.Inscriptions {
	case InscriptionPremiere, InscriptionOuverte, InscriptionFermee:
	default:
		errs = append(errs, errors.New("FC_INSCRIPTIONS : premiere, ouverte ou fermee"))
	}

	if err := c.NiveauLogs.UnmarshalText([]byte(val("FC_NIVEAU_LOGS", "info"))); err != nil {
		errs = append(errs, errors.New("FC_NIVEAU_LOGS : debug, info, warn ou error"))
	}
	if c.FormatLogs != "json" && c.FormatLogs != "texte" {
		errs = append(errs, errors.New("FC_FORMAT_LOGS : json ou texte"))
	}

	if c.SauvegardeA == "non" {
		c.SauvegardeA = ""
	} else if _, err := time.Parse("15:04", c.SauvegardeA); err != nil {
		errs = append(errs, errors.New("FC_SAUVEGARDE_HEURE : heure HH:MM (ex. 03:30) ou « non »"))
	}
	n, err := strconv.Atoi(val("FC_SAUVEGARDE_GARDER", "7"))
	if err != nil || n < 1 || n > 365 {
		errs = append(errs, errors.New("FC_SAUVEGARDE_GARDER : nombre entre 1 et 365"))
	}
	c.SauvegardeNb = n

	j, err := strconv.Atoi(val("FC_DUREE_SESSION_JOURS", "90"))
	if err != nil || j < 1 || j > 400 {
		errs = append(errs, errors.New("FC_DUREE_SESSION_JOURS : nombre entre 1 et 400"))
	}
	c.DureeSession = time.Duration(j) * 24 * time.Hour

	jm, err := strconv.Atoi(val("FC_SESSION_MAX_JOURS", "365"))
	if err != nil || jm < 1 || jm > 3650 || jm < j {
		errs = append(errs, errors.New("FC_SESSION_MAX_JOURS : nombre entre FC_DUREE_SESSION_JOURS et 3650"))
	}
	c.SessionMax = time.Duration(jm) * 24 * time.Hour

	if c.ContactPush == "" {
		c.ContactPush = "mailto:admin@" + u.Hostname()
	}
	return c, errors.Join(errs...)
}
