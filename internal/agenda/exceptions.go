// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Fois annulées : un événement qui revient (judo le mardi) peut être annulé
// une seule fois (vacances, cours annulé) sans toucher aux autres.

// exceptionsMax : fois annulées par événement, au plus.
const exceptionsMax = 200

// exceptions : les fois annulées de ces événements (id → jour → vrai).
func (m *Module) exceptions(ctx context.Context, ids []int64) (map[int64]map[string]bool, error) {
	out := map[int64]map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	l, err := m.a.Q.ExceptionsDesEvenements(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, x := range l {
		if out[x.EvenementID] == nil {
			out[x.EvenementID] = map[string]bool{}
		}
		out[x.EvenementID][x.Jour] = true
	}
	return out, nil
}

// deplierSans : comme deplier, sans les fois annulées.
func deplierSans(e donnees.Evenement, lieu *time.Location, debut, fin time.Time, annulees map[string]bool) [][2]time.Time {
	l := deplier(e, lieu, debut, fin)
	if len(annulees) == 0 {
		return l
	}
	out := l[:0]
	for _, d := range l {
		if !annulees[repetition.CleJour(d[0])] {
			out = append(out, d)
		}
	}
	return out
}

// sauter : « Annuler cette fois seulement ».
func (m *Module) sauter(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	m.exception(w, r, c, true)
}

// retablir : la fois annulée a lieu finalement.
func (m *Module) retablir(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	m.exception(w, r, c, false)
}

func (m *Module) exception(w http.ResponseWriter, r *http.Request, c *appli.Contexte, annuler bool) {
	ctx := r.Context()
	e, err := m.evenement(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Événement introuvable.")
		return
	}
	if !modifiable(c, e) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul un parent (ou qui l'a créé) peut le modifier.")
		return
	}
	jour, err := time.ParseInLocation("2006-01-02", r.PostFormValue("jour"), c.Lieu)
	if err != nil || e.Repetition == "" {
		m.a.Echec(w, r, http.StatusBadRequest, "Date invalide.")
		return
	}
	cle := repetition.CleJour(jour)
	if annuler {
		// La date doit être une vraie fois de l'événement.
		if len(deplier(e, c.Lieu, jour, jour.AddDate(0, 0, 1))) == 0 {
			m.a.Echec(w, r, http.StatusBadRequest, "L'événement n'a pas lieu ce jour-là.")
			return
		}
		if n, err := m.a.Q.CompterExceptions(ctx, e.ID); err != nil || n >= exceptionsMax {
			m.a.Echec(w, r, http.StatusBadRequest, "Trop de fois annulées pour cet événement.")
			return
		}
		err = m.a.Q.AjouterException(ctx, donnees.AjouterExceptionParams{EvenementID: e.ID, Jour: cle})
	} else {
		err = m.a.Q.RetirerException(ctx, donnees.RetirerExceptionParams{EvenementID: e.ID, Jour: cle})
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	if annuler {
		m.a.Flash(w, "« "+e.Titre+" » annulé le "+appli.DateLongue(jour))
		m.a.Rediriger(w, r, "/agenda?date="+cle)
		return
	}
	m.a.Flash(w, "« "+e.Titre+" » rétabli le "+appli.DateLongue(jour))
	m.a.Rediriger(w, r, "/agenda/"+strconv.FormatInt(e.ID, 10))
}
