// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Agenda dans le calendrier du téléphone (iPhone, Android, Thunderbird…) :
// chaque membre peut créer un lien secret vers un flux iCalendar (RFC 5545)
// de l'agenda de la famille. Le téléphone le relit tout seul et fait ses
// propres rappels. Seule l'empreinte du lien est en base ; le recréer rend
// l'ancien inutile, le couper le supprime. Lecture seule : rien ne peut être
// modifié par ce lien.

// Période du flux : 3 mois en arrière, 1 an en avant.
const (
	icalMoisAvant = 3
	icalMoisApres = 12
)

type vueCalendrier struct {
	Lien     string // lien qui vient d'être créé (montré une seule fois)
	Webcal   string // le même en webcal:// (ouvre directement l'appli Calendrier)
	Existe   bool
	CreeLe   string
	LuLe     string
	Securise bool // adresse en https (un lien en http circule en clair)
}

func (m *Module) pageCalendrier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	m.afficherCalendrier(w, r, c, "")
}

func (m *Module) afficherCalendrier(w http.ResponseWriter, r *http.Request, c *appli.Contexte, jeton string) {
	v := vueCalendrier{Securise: m.a.Config.URLPublique.Scheme == "https"}
	if x, err := m.a.Q.IcalMembre(r.Context(), c.Membre.ID); err == nil {
		v.Existe = true
		v.CreeLe = appli.DateLongue(c.Heure(x.CreeLe))
		if x.LuLe > 0 {
			t := c.Heure(x.LuLe)
			v.LuLe = appli.DateLongue(t) + " à " + t.Format("15:04")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if jeton != "" {
		u := *m.a.Config.URLPublique
		u.Path = "/ical/" + jeton + ".ics"
		v.Lien = u.String()
		u.Scheme = "webcal"
		v.Webcal = u.String()
	}
	m.a.Afficher(w, r, c, "agenda/calendrier", "Agenda dans le téléphone", mod, v)
}

// creerLien : un nouveau lien secret (l'ancien ne marche plus).
func (m *Module) creerLien(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	jeton, empreinte := appli.NouveauJeton()
	if err := m.a.Q.PoserIcal(r.Context(), donnees.PoserIcalParams{MembreID: c.Membre.ID, FamilleID: c.Famille.ID,
		Empreinte: empreinte, CreeLe: time.Now().Unix()}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Journal.Info("lien d'agenda créé", "membre", c.Membre.ID)
	// Pas de redirection : le lien n'est montré qu'ici, une seule fois.
	m.afficherCalendrier(w, r, c, jeton)
}

// couperLien : le lien ne marche plus.
func (m *Module) couperLien(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	if err := m.a.Q.SupprimerIcal(r.Context(), c.Membre.ID); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Lien coupé : le téléphone ne recevra plus l'agenda")
	m.a.Rediriger(w, r, "/agenda/calendrier")
}

// flux : le fichier .ics (sans session : le lien secret suffit).
func (m *Module) flux(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jeton := strings.TrimSuffix(r.PathValue("jeton"), ".ics")
	if b, err := base64.RawURLEncoding.DecodeString(jeton); err != nil || len(b) < 16 {
		http.NotFound(w, r)
		return
	}
	lien, err := m.a.Q.IcalParEmpreinte(ctx, appli.Empreinte256(jeton))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := m.a.Q.FamilleParID(ctx, lien.FamilleID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !strings.Contains(","+f.Modules+",", ","+mod+",") {
		http.NotFound(w, r) // agenda désactivé
		return
	}
	lieu, err := time.LoadLocation(f.Fuseau)
	if err != nil {
		lieu = time.UTC
	}
	maintenant := time.Now().In(lieu)
	debut := minuit(maintenant).AddDate(0, -icalMoisAvant, 0)
	fin := minuit(maintenant).AddDate(0, icalMoisApres, 0)
	evts, err := m.a.Q.EvenementsPeriode(ctx, donnees.EvenementsPeriodeParams{FamilleID: f.ID, FinPeriode: fin.Unix(),
		DebutPeriode: debut.Unix(), JourMin: debut.AddDate(-1, 0, -1).Format("2006-01-02")})
	if err != nil {
		m.a.Journal.Error("agenda : flux iCal", "erreur", err)
		http.Error(w, "erreur", http.StatusInternalServerError)
		return
	}
	ids := make([]int64, len(evts))
	for i := range evts {
		ids[i] = evts[i].ID
	}
	annulees, err := m.exceptions(ctx, ids)
	if err != nil {
		http.Error(w, "erreur", http.StatusInternalServerError)
		return
	}
	var b bytes.Buffer
	l := func(s string) { plierIcal(&b, s) }
	l("BEGIN:VCALENDAR")
	l("VERSION:2.0")
	l("PRODID:-//FamilyCocon//Agenda//FR")
	l("CALSCALE:GREGORIAN")
	l("X-WR-CALNAME:" + echapperIcal(f.Nom))
	l("X-WR-TIMEZONE:" + f.Fuseau)
	l("REFRESH-INTERVAL;VALUE=DURATION:PT1H")
	l("X-PUBLISHED-TTL:PT1H")
	horodatage := time.Now().UTC().Format("20060102T150405Z")
	for _, e := range evts {
		for _, d := range deplierSans(e, lieu, debut, fin, annulees[e.ID]) {
			l("BEGIN:VEVENT")
			l("UID:" + strconv.FormatInt(e.ID, 10) + "-" + d[0].Format("20060102") + "@familycocon")
			l("DTSTAMP:" + horodatage)
			if e.Journee == 1 {
				l("DTSTART;VALUE=DATE:" + d[0].Format("20060102"))
				l("DTEND;VALUE=DATE:" + minuit(d[1]).AddDate(0, 0, 1).Format("20060102"))
			} else {
				l("DTSTART:" + d[0].UTC().Format("20060102T150405Z"))
				l("DTEND:" + d[1].UTC().Format("20060102T150405Z"))
			}
			l("SUMMARY:" + echapperIcal(e.Titre))
			if e.Lieu != "" {
				l("LOCATION:" + echapperIcal(e.Lieu))
			}
			if e.Note != "" {
				l("DESCRIPTION:" + echapperIcal(e.Note))
			}
			// « Me le rappeler » : le téléphone prévient une heure avant
			// (la veille au soir pour une journée entière).
			if e.Rappel == 1 {
				l("BEGIN:VALARM")
				l("ACTION:DISPLAY")
				l("DESCRIPTION:" + echapperIcal(e.Titre))
				if e.Journee == 1 {
					l("TRIGGER:-PT6H")
				} else {
					l("TRIGGER:-PT1H")
				}
				l("END:VALARM")
			}
			l("END:VEVENT")
		}
	}
	l("END:VCALENDAR")
	_ = m.a.Q.NoterLectureIcal(ctx, donnees.NoterLectureIcalParams{LuLe: time.Now().Unix(), MembreID: lien.MembreID})
	h := w.Header()
	h.Set("Content-Type", "text/calendar; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Disposition", `inline; filename="familycocon.ics"`)
	_, _ = w.Write(b.Bytes())
}

// echapperIcal : \ , ; et retours à la ligne, comme le veut la norme.
func echapperIcal(s string) string {
	return strings.NewReplacer(`\`, `\\`, ",", `\,`, ";", `\;`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`).Replace(s)
}

// plierIcal : lignes de 75 octets au plus, la suite commence par une espace
// (sans couper un caractère en deux). Même règle que les vCard (contacts).
func plierIcal(b *bytes.Buffer, ligne string) {
	limite := 75
	for len(ligne) > limite {
		n := limite
		for n > 0 && !utf8.RuneStart(ligne[n]) {
			n--
		}
		b.WriteString(ligne[:n] + "\r\n ")
		ligne, limite = ligne[n:], 74
	}
	b.WriteString(ligne + "\r\n")
}
