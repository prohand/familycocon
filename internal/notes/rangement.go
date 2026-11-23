// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/riche"
)

// Listes : ranger les éléments, cochés en bas, rappel.
//   - « ✏️ Ranger la liste » (ceux qui peuvent modifier la note) : chaque
//     élément devient un champ : ✓ enregistrer, ↑ ↓ déplacer, ✕ supprimer.
//     Chaque action dit la ligne telle qu'elle était (« avant ») : si la note
//     a changé entre-temps, rien n'est touché.
//   - « Cochés en bas » : à l'affichage seulement (le texte ne bouge pas :
//     décocher remet l'élément à sa place). Réglage de la note.
//   - Rappel : une date et une heure ; une notification part alors à ceux
//     qui voient la note (son auteur seul pour une note privée), même s'ils
//     ont l'appli ouverte, une seule fois.

// routesRangement : appelées par Brancher.
func (m *Module) routesRangement() {
	a := m.a
	a.Route("GET /notes/liste/{id}", mod, appli.Membre, m.cases) // (pas /notes/{id}/… : conflit avec /notes/photos/{id})
	a.Route("POST /notes/elements/{id}/{ligne}", mod, appli.Membre, m.elementModifier)
	a.Route("POST /notes/elements/{id}/{ligne}/supprimer", mod, appli.Membre, m.elementSupprimer)
	a.Route("POST /notes/elements/{id}/{ligne}/monter", mod, appli.Membre, m.elementMonter)
	a.Route("POST /notes/elements/{id}/{ligne}/descendre", mod, appli.Membre, m.elementDescendre)
	a.Route("POST /notes/{id}/cochees", mod, appli.Membre, m.cocheesEnBas)
	a.Route("POST /notes/{id}/rappel", mod, appli.Membre, m.rappel)
	a.AjouterMinute(m.envoyerRappels)
}

// cases : le bloc de la liste (rechargé seul), en mode « Ranger » ou non.
func (m *Module) cases(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return
	}
	m.a.Fragment(w, r, c, "notes/note", "cases-note", vueCases(c, n, r.URL.Query().Get("ranger") == "1" && modifiable(c, n)))
}

// element : la note modifiable, ses lignes et la case visée (vérifiée :
// la ligne n'a pas changé depuis l'affichage). ok faux : réponse déjà faite.
func (m *Module) element(w http.ResponseWriter, r *http.Request, c *appli.Contexte) (donnees.Note, []string, int, bool) {
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return n, nil, 0, false
	}
	if !modifiable(c, n) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul son auteur (ou un parent) peut ranger cette liste.")
		return n, nil, 0, false
	}
	lignes := strings.Split(n.Texte, "\n")
	i, err := strconv.Atoi(r.PathValue("ligne"))
	if err != nil || i < 0 || i >= len(lignes) || strings.TrimRight(lignes[i], "\r") != r.PostFormValue("avant") {
		m.a.Echec(w, r, http.StatusConflict, "La liste vient de changer : rechargez la page.")
		return n, nil, 0, false
	}
	if _, _, _, ok := lireCase(strings.TrimRight(lignes[i], "\r")); !ok {
		m.a.Echec(w, r, http.StatusBadRequest, "Ce n'est pas un élément de la liste.")
		return n, nil, 0, false
	}
	return n, lignes, i, true
}

// ecrire : le nouveau texte, puis le bloc en mode « Ranger ».
func (m *Module) ecrire(w http.ResponseWriter, r *http.Request, c *appli.Contexte, n donnees.Note, lignes []string) {
	nouveau := strings.Join(lignes, "\n")
	if ok, err := m.changerTexte(r, c, n, nouveau); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if !ok {
		m.a.Echec(w, r, http.StatusConflict, "La liste vient de changer : rechargez la page.")
		return
	}
	n.Texte = nouveau
	if !appli.EstHtmx(r) {
		m.a.Rediriger(w, r, "/notes/"+strconv.FormatInt(n.ID, 10))
		return
	}
	m.a.Fragment(w, r, c, "notes/note", "cases-note", vueCases(c, n, true))
}

// elementModifier : le texte d'un élément (texte simple : ses signes de
// mise en forme sont gardés tels quels ; inchangé, rien ne bouge).
func (m *Module) elementModifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, lignes, i, ok := m.element(w, r, c)
	if !ok {
		return
	}
	t := strings.TrimSpace(r.PostFormValue("texte"))
	if t == "" || utf8.RuneCountInString(t) > elementMax {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Un élément : de 1 à %d caractères.", elementMax))
		return
	}
	prefixe, ancien, cochee, _ := lireCase(strings.TrimRight(lignes[i], "\r"))
	if t != riche.LigneSimple(ancien) {
		marque := "[ ] "
		if cochee {
			marque = "[x] "
		}
		lignes[i] = prefixe + marque + riche.Echapper(t)
	}
	m.ecrire(w, r, c, n, lignes)
}

func (m *Module) elementSupprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, lignes, i, ok := m.element(w, r, c)
	if !ok {
		return
	}
	m.ecrire(w, r, c, n, append(lignes[:i], lignes[i+1:]...))
}

func (m *Module) elementMonter(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	m.deplacer(w, r, c, -1)
}

func (m *Module) elementDescendre(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	m.deplacer(w, r, c, 1)
}

// deplacer : l'élément échange sa place avec l'élément voisin (seulement
// dans la même suite de cases : il ne saute pas un paragraphe).
func (m *Module) deplacer(w http.ResponseWriter, r *http.Request, c *appli.Contexte, sens int) {
	n, lignes, i, ok := m.element(w, r, c)
	if !ok {
		return
	}
	j := i + sens
	if j >= 0 && j < len(lignes) {
		if _, _, _, voisin := lireCase(strings.TrimRight(lignes[j], "\r")); voisin {
			lignes[i], lignes[j] = lignes[j], lignes[i]
		}
	}
	m.ecrire(w, r, c, n, lignes)
}

// cocheesEnBas : le réglage de la note (« en_bas » = 1 ou 0).
func (m *Module) cocheesEnBas(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return
	}
	if !modifiable(c, n) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul son auteur (ou un parent) peut changer ce réglage.")
		return
	}
	n.CocheesEnBas = 0
	if r.PostFormValue("en_bas") == "1" {
		n.CocheesEnBas = 1
	}
	if err := m.a.Q.ChangerCocheesEnBas(r.Context(), donnees.ChangerCocheesEnBasParams{EnBas: n.CocheesEnBas, ID: n.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.repondreCases(w, r, c, n)
}

// Format du champ « date et heure » du navigateur.
const formatRappel = "2006-01-02T15:04"

// rappel : la date et l'heure du rappel (vide, ou « retirer » : plus de rappel).
func (m *Module) rappel(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return
	}
	if !modifiable(c, n) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul son auteur (ou un parent) peut y mettre un rappel.")
		return
	}
	var quand int64
	if v := strings.TrimSpace(r.PostFormValue("rappel")); v != "" && r.PostFormValue("retirer") == "" {
		t, err := time.ParseInLocation(formatRappel, v, c.Lieu)
		maintenant := c.Maintenant()
		if err != nil || t.Before(maintenant.Add(-time.Minute)) || t.After(maintenant.AddDate(2, 0, 0)) {
			m.a.Echec(w, r, http.StatusBadRequest, "Choisissez une date et une heure à venir (2 ans au plus).")
			return
		}
		quand = t.Unix()
	}
	if err := m.a.Q.ChangerRappelNote(r.Context(), donnees.ChangerRappelNoteParams{RappelLe: quand, ID: n.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	if quand == 0 {
		m.a.Flash(w, "Rappel retiré")
	} else {
		m.a.Flash(w, "Rappel le "+quandRappel(time.Unix(quand, 0).In(c.Lieu), c.Maintenant())+" ⏰")
	}
	m.a.Rediriger(w, r, "/notes/"+strconv.FormatInt(n.ID, 10))
}

// quandRappel : « aujourd'hui à 18:00 », « demain à 9:30 », « vendredi 28/11 à 18:00 ».
func quandRappel(t, maintenant time.Time) string {
	h := t.Format("15:04")
	j := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	a := time.Date(maintenant.Year(), maintenant.Month(), maintenant.Day(), 0, 0, 0, 0, t.Location())
	switch int(j.Sub(a).Hours() / 24) {
	case 0:
		return "aujourd'hui à " + h
	case 1:
		return "demain à " + h
	}
	return appli.JourFR(t) + " " + t.Format("02/01") + " à " + h
}

// envoyerRappels : chaque minute, les rappels arrivés à leur heure.
func (m *Module) envoyerRappels(ctx context.Context, maintenant time.Time) {
	l, err := m.a.Q.NotesARappeler(ctx, maintenant.Unix())
	if err != nil {
		m.a.Journal.Error("rappels des notes", "erreur", err)
		return
	}
	for _, n := range l {
		var pour []int64
		if n.Privee == 1 && n.AuteurID.Valid {
			pour = []int64{n.AuteurID.Int64}
		}
		// Marqué envoyé d'abord : jamais deux fois (même si l'envoi échoue).
		if err := m.a.Q.RappelNoteEnvoye(ctx, n.ID); err != nil {
			m.a.Journal.Error("rappel d'une note", "note", n.ID, "erreur", err)
			continue
		}
		m.a.Rappeler(ctx, n.FamilleID, mod, "⏰ "+apercu(n.Titre, n.Texte), "/notes/"+strconv.FormatInt(n.ID, 10), pour)
		m.a.Journal.Info("rappel d'une note envoyé", "note", n.ID)
	}
}
