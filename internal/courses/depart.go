// SPDX-License-Identifier: GPL-3.0-or-later

package courses

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"familycocon/internal/appli"
)

// « Je pars faire les courses » : un toucher prévient toute la famille
// (sauf soi), avec le lien de la liste, pour qu'elle ajoute vite ce qui
// manque. C'est un appel voulu (appli.Appeler) : il arrive même chez ceux
// qui ont coupé les notifications des courses. Garde-fou : un seul appel
// toutes les 30 minutes par liste (donc par famille et par liste), gardé en
// mémoire (un redémarrage du serveur l'oublie, sans conséquence).

// delaiDepart : temps minimum entre deux appels pour la même liste.
const delaiDepart = 30 * time.Minute

// departs : le dernier appel de chaque liste.
type departs struct {
	mu    sync.Mutex
	liste map[int64]depart // clé : identifiant de la liste
}

type depart struct {
	le  time.Time
	qui string // prénom de celui qui est parti
}

// reserver : vrai si l'appel peut partir (il est alors noté) ; sinon,
// l'appel précédent. Les appels trop vieux sont oubliés au passage : la
// mémoire ne grossit pas.
func (d *departs) reserver(liste int64, qui string, maintenant time.Time) (depart, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.liste == nil {
		d.liste = map[int64]depart{}
	}
	for id, x := range d.liste {
		if maintenant.Sub(x.le) >= delaiDepart {
			delete(d.liste, id)
		}
	}
	if x, ok := d.liste[liste]; ok {
		return x, false
	}
	d.liste[liste] = depart{le: maintenant, qui: qui}
	return depart{}, true
}

func (m *Module) partir(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	liste, err := m.liste(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Liste introuvable.")
		return
	}
	retour := fmt.Sprintf("/courses/liste/%d", liste.ID)
	avant, ok := m.departs.reserver(liste.ID, c.Membre.Nom, time.Now())
	if !ok {
		minutes := int(time.Since(avant.le).Minutes())
		quand := "à l'instant"
		if minutes >= 1 {
			quand = "il y a " + appli.Pluriel(minutes, "minute", "minutes")
		}
		message := fmt.Sprintf("Déjà annoncé %s par %s : un seul message toutes les 30 minutes.", quand, avant.qui)
		m.repondreDepart(w, r, retour, message)
		return
	}
	// Plusieurs listes : on dit laquelle (« Drive »).
	texte := fmt.Sprintf("%s part faire les courses : ajoutez vite ce qui manque 🛒", c.Membre.Nom)
	if n, err := m.a.Q.CompterListes(r.Context(), c.Famille.ID); err == nil && n > 1 {
		texte = fmt.Sprintf("%s part faire les courses (%s) : ajoutez vite ce qui manque 🛒", c.Membre.Nom, liste.Nom)
	}
	m.a.Appeler(c, "courses", texte, retour)
	m.repondreDepart(w, r, retour, "C'est envoyé : la famille est prévenue 🛒")
}

// repondreDepart : un petit message (htmx) ou la liste avec le message.
func (m *Module) repondreDepart(w http.ResponseWriter, r *http.Request, retour, message string) {
	if appli.EstHtmx(r) {
		appli.Notifier(w, message, false)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	m.a.Flash(w, message)
	http.Redirect(w, r, retour, http.StatusSeeOther)
}
