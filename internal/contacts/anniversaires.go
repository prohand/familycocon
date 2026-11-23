// SPDX-License-Identifier: GPL-3.0-or-later

package contacts

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// --- Dates de naissance ---------------------------------------------------------

// Naissance : une date de naissance (année 0 : inconnue).
type Naissance struct {
	Annee, Mois, Jour int
}

// LireNaissance : « 2018-03-05 » ou « 0000-03-05 » (année inconnue).
func LireNaissance(s string) (Naissance, bool) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return Naissance{}, false
	}
	a, e1 := strconv.Atoi(s[:4])
	m, e2 := strconv.Atoi(s[5:7])
	j, e3 := strconv.Atoi(s[8:])
	if e1 != nil || e2 != nil || e3 != nil || m < 1 || m > 12 || j < 1 || j > 31 {
		return Naissance{}, false
	}
	// Le jour doit exister (29 février accepté quand l'année est inconnue).
	ref := a
	if ref == 0 {
		ref = 2000
	}
	if time.Date(ref, time.Month(m), j, 0, 0, 0, 0, time.UTC).Day() != j {
		return Naissance{}, false
	}
	return Naissance{Annee: a, Mois: m, Jour: j}, true
}

// Prochain : le prochain anniversaire à partir du jour auj (inclus). Né un
// 29 février : fêté le 28 les années sans 29.
func (n Naissance) Prochain(auj time.Time) time.Time {
	date := func(annee int) time.Time {
		j := n.Jour
		if n.Mois == 2 && j == 29 && time.Date(annee, 2, 29, 0, 0, 0, 0, auj.Location()).Day() != 29 {
			j = 28
		}
		return time.Date(annee, time.Month(n.Mois), j, 0, 0, 0, 0, auj.Location())
	}
	d := date(auj.Year())
	if d.Before(auj) {
		d = date(auj.Year() + 1)
	}
	return d
}

// Age : l'âge atteint à la date d (0 si l'année est inconnue).
func (n Naissance) Age(d time.Time) int {
	if n.Annee == 0 {
		return 0
	}
	return d.Year() - n.Annee
}

// Anniversaire : un anniversaire à venir (contact ou membre).
type Anniversaire struct {
	Nom    string
	Date   time.Time
	Age    int
	Quand  string
	URL    string
	Membre *donnees.Membre // nil pour un contact
	Idees  string
	Proche bool
}

// anniversaires : contacts et membres, du plus proche au plus lointain.
// c : le contexte de la requête s'il y en a un (les membres déjà chargés
// pour la page resservent), nil pour les rappels de fond.
func (m *Module) anniversaires(ctx context.Context, c *appli.Contexte, f donnees.Famille, auj time.Time) ([]Anniversaire, error) {
	var out []Anniversaire
	contacts, err := m.a.Q.ContactsAvecNaissance(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	for _, ct := range contacts {
		n, ok := LireNaissance(ct.Naissance)
		if !ok {
			continue
		}
		d := n.Prochain(auj)
		out = append(out, Anniversaire{Nom: ct.Nom, Date: d, Age: n.Age(d), URL: fmt.Sprintf("/contacts/%d", ct.ID), Idees: ct.Idees})
	}
	var membres []donnees.Membre
	if c != nil && c.Famille.ID == f.ID {
		membres, err = m.a.Membres(ctx, c)
	} else {
		membres, err = m.a.Q.MembresDeFamille(ctx, f.ID)
	}
	if err != nil {
		return nil, err
	}
	for i := range membres {
		n, ok := LireNaissance(membres[i].Naissance)
		if !ok {
			continue
		}
		d := n.Prochain(auj)
		out = append(out, Anniversaire{Nom: membres[i].Nom, Date: d, Age: n.Age(d), URL: "/anniversaires", Membre: &membres[i]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	for i := range out {
		out[i].Quand = appli.Quand(out[i].Date, auj)
		out[i].Proche = out[i].Date.Sub(auj) <= 7*24*time.Hour
	}
	return out, nil
}

func minuit(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func titreAnniv(a Anniversaire) string {
	if a.Age > 0 {
		return fmt.Sprintf("Anniversaire de %s (%d ans)", a.Nom, a.Age)
	}
	return "Anniversaire de " + a.Nom
}

// echeances : les anniversaires de la période (rappels).
func (m *Module) echeances(ctx context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
	l, err := m.anniversaires(ctx, appli.ContexteDe(ctx), f, debut)
	if err != nil {
		return nil, err
	}
	var out []appli.Echeance
	for _, a := range l {
		if a.Date.Before(fin) {
			out = append(out, appli.Echeance{Jour: a.Date, Titre: titreAnniv(a), Emoji: "🎂", URL: a.URL, Module: modAnniv})
		}
	}
	return out, nil
}

type moisAnniv struct {
	Mois  string
	Liste []Anniversaire
}

func (m *Module) pageAnniversaires(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	auj := minuit(c.Maintenant())
	l, err := m.anniversaires(r.Context(), c, c.Famille, auj)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	var parMois []moisAnniv
	for _, a := range l {
		nom := appli.MoisFR(a.Date.Month())
		nom = strings.ToUpper(nom[:1]) + nom[1:]
		if a.Date.Year() != auj.Year() {
			nom += " " + strconv.Itoa(a.Date.Year())
		}
		if len(parMois) == 0 || parMois[len(parMois)-1].Mois != nom {
			parMois = append(parMois, moisAnniv{Mois: nom})
		}
		parMois[len(parMois)-1].Liste = append(parMois[len(parMois)-1].Liste, a)
	}
	m.a.Afficher(w, r, c, "contacts/anniversaires", "Anniversaires", modAnniv, parMois)
}

func (m *Module) resumeAnniv(ctx context.Context, c *appli.Contexte) appli.Resume {
	auj := minuit(c.Maintenant())
	l, err := m.anniversaires(ctx, c, c.Famille, auj)
	if err != nil || len(l) == 0 {
		return appli.Resume{Info: "Aucune date"}
	}
	r := appli.Resume{Info: l[0].Nom + " : " + strings.ToLower(l[0].Quand[:1]) + l[0].Quand[1:]}
	if l[0].Proche {
		r.Badge = 1
	}
	return r
}
