// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Listes à cocher dans une note : une ligne qui commence par « [ ] » (ou
// « - [ ] ») est une case ; « [x] » une case cochée. Tout le monde peut
// cocher une note qu'il voit (liste de valises, de cadeaux…).

// CaseNote : une case de la note.
type CaseNote struct {
	Ligne  int // numéro de la ligne dans le texte
	Texte  string
	Cochee bool
}

// lireCase : la ligne est-elle une case ? (préfixe, texte, cochée)
func lireCase(l string) (prefixe, texte string, cochee, ok bool) {
	t := strings.TrimLeft(l, " \t")
	debut := l[:len(l)-len(t)]
	if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
		debut += t[:2]
		t = t[2:]
	}
	switch {
	case strings.HasPrefix(t, "[ ]"):
		return debut, strings.TrimSpace(t[3:]), false, true
	case strings.HasPrefix(t, "[x]"), strings.HasPrefix(t, "[X]"):
		return debut, strings.TrimSpace(t[3:]), true, true
	}
	return "", "", false, false
}

// Cases : les cases du texte (vide : pas de liste à cocher).
func Cases(texte string) []CaseNote {
	var out []CaseNote
	for i, l := range strings.Split(texte, "\n") {
		if _, t, cochee, ok := lireCase(strings.TrimRight(l, "\r")); ok {
			out = append(out, CaseNote{Ligne: i, Texte: t, Cochee: cochee})
		}
	}
	return out
}

// basculer : le texte avec la case de la ligne i dans l'état voulu (ok =
// faux si la ligne n'est pas une case).
func basculer(texte string, i int, cochee bool) (string, bool) {
	lignes := strings.Split(texte, "\n")
	if i < 0 || i >= len(lignes) {
		return texte, false
	}
	prefixe, t, _, ok := lireCase(strings.TrimRight(lignes[i], "\r"))
	if !ok {
		return texte, false
	}
	marque := "[ ] "
	if cochee {
		marque = "[x] "
	}
	lignes[i] = prefixe + marque + t
	return strings.Join(lignes, "\n"), true
}

// cocher : une case cochée ou décochée (l'état voulu est envoyé : deux
// personnes qui cochent en même temps ne s'annulent pas).
func (m *Module) cocher(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return
	}
	i, err := strconv.Atoi(r.PostFormValue("ligne"))
	if err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, "Case inconnue.")
		return
	}
	nouveau, ok := basculer(n.Texte, i, r.PostFormValue("cochee") == "1")
	if !ok {
		m.a.Echec(w, r, http.StatusBadRequest, "Case inconnue.")
		return
	}
	if nouveau != n.Texte {
		k, err := m.a.Q.CocherNote(r.Context(), donnees.CocherNoteParams{Texte: nouveau, ModifieLe: max(time.Now().Unix(), n.ModifieLe+1),
			ID: n.ID, FamilleID: c.Famille.ID, Ancien: n.Texte})
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		if k == 0 {
			m.a.Echec(w, r, http.StatusConflict, "La note vient de changer : rechargez la page.")
			return
		}
		m.a.Publier(c, mod, "")
		n.Texte = nouveau
	}
	if !appli.EstHtmx(r) {
		m.a.Rediriger(w, r, "/notes/"+strconv.FormatInt(n.ID, 10))
		return
	}
	m.a.Fragment(w, r, c, "notes/note", "cases-note", map[string]any{"ID": n.ID, "Cases": Cases(n.Texte)})
}
