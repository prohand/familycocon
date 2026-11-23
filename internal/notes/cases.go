// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"familycocon/internal/riche"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

// lireCase : voir riche.LireCase.
var lireCase = riche.LireCase

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

// apercuCases : le début du texte pour la carte de la liste des notes, les
// cases dessinées (« [ ] lait » → « ☐ lait », « [x] pain » → « ☑ pain »).
func apercuCases(texte string) string {
	lignes := strings.Split(texte, "\n")
	for i, l := range lignes {
		lignes[i] = riche.LigneSimple(l)
	}
	return strings.Join(lignes, "\n")
}

// sansCasesVides : le texte sans les cases qui n'ont pas de texte.
func sansCasesVides(texte string) string {
	var garde []string
	for _, l := range strings.Split(texte, "\n") {
		if _, t, _, ok := lireCase(strings.TrimRight(l, "\r")); ok && t == "" {
			continue
		}
		garde = append(garde, l)
	}
	return strings.Join(garde, "\n")
}

// elementMax : longueur d'un élément ajouté à une liste.
const elementMax = 200

// ajouterCase : le texte avec une case de plus, juste après la dernière
// case de la note (la liste reste d'un seul tenant), ou à la fin.
func ajouterCase(texte, element string) string {
	ligne := "[ ] " + element
	lignes := strings.Split(texte, "\n")
	derniere := -1
	for i, l := range lignes {
		if _, _, _, ok := lireCase(strings.TrimRight(l, "\r")); ok {
			derniere = i
		}
	}
	if derniere < 0 {
		if strings.TrimSpace(texte) == "" {
			return ligne
		}
		return strings.TrimRight(texte, "\n") + "\n" + ligne
	}
	// Même retrait et même puce (« - ») que la case d'avant.
	prefixe, _, _, _ := lireCase(strings.TrimRight(lignes[derniere], "\r"))
	lignes = append(lignes[:derniere+1], append([]string{prefixe + ligne}, lignes[derniere+1:]...)...)
	return strings.Join(lignes, "\n")
}

// retirerCochees : le texte sans les cases cochées (rend aussi leur nombre).
func retirerCochees(texte string) (string, int) {
	var garde []string
	n := 0
	for _, l := range strings.Split(texte, "\n") {
		if _, _, cochee, ok := lireCase(strings.TrimRight(l, "\r")); ok && cochee {
			n++
			continue
		}
		garde = append(garde, l)
	}
	return strings.Join(garde, "\n"), n
}

// changerTexte : écrit le nouveau texte de la note, sauf si elle a changé
// entre-temps (ok = faux : on prévient au lieu d'écraser).
func (m *Module) changerTexte(r *http.Request, c *appli.Contexte, n donnees.Note, nouveau string) (ok bool, err error) {
	if nouveau == n.Texte {
		return true, nil
	}
	k, err := m.a.Q.CocherNote(r.Context(), donnees.CocherNoteParams{Texte: nouveau, ModifieLe: max(time.Now().Unix(), n.ModifieLe+1),
		ID: n.ID, FamilleID: c.Famille.ID, Ancien: n.Texte})
	if err != nil || k == 0 {
		return false, err
	}
	m.a.Publier(c, mod, "")
	return true, nil
}

// repondreCases : la liste rechargée seule (htmx), sinon la page de la note.
func (m *Module) repondreCases(w http.ResponseWriter, r *http.Request, c *appli.Contexte, n donnees.Note) {
	if !appli.EstHtmx(r) {
		m.a.Rediriger(w, r, "/notes/"+strconv.FormatInt(n.ID, 10))
		return
	}
	m.a.Fragment(w, r, c, "notes/note", "cases-note", vueCases(c, n, false))
}

// vueCases : ce que montre le bloc de la liste à cocher (toute la note :
// le texte et les photos autour des cases).
func vueCases(c *appli.Contexte, n donnees.Note, ranger bool) map[string]any {
	cases := Cases(n.Texte)
	cochees := 0
	for _, x := range cases {
		if x.Cochee {
			cochees++
		}
	}
	return map[string]any{"ID": n.ID, "Cases": cases, "HTML": riche.HTML(n.Texte, riche.Options{Mode: riche.Lecture, Note: n.ID, CocheesEnBas: n.CocheesEnBas == 1, Ranger: ranger, Jeton: c.CSRF}),
		"Cochees": cochees, "Modifiable": modifiable(c, n), "CSRF": c.CSRF, "Ranger": ranger, "EnBas": n.CocheesEnBas == 1, "Total": len(cases)}
}

// ajouterElement : un élément de plus dans la liste (tous ceux qui voient
// la note, comme pour cocher : liste de courses, de valises…).
func (m *Module) ajouterElement(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return
	}
	element := strings.Join(strings.Fields(r.PostFormValue("element")), " ")
	if element == "" || utf8.RuneCountInString(element) > elementMax {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Écrivez l'élément (%d caractères au plus).", elementMax))
		return
	}
	nouveau := ajouterCase(n.Texte, riche.Echapper(element)) // texte simple : signes gardés tels quels
	if utf8.RuneCountInString(nouveau) > 10000 {
		m.a.Echec(w, r, http.StatusBadRequest, "Note trop longue (10 000 caractères au plus).")
		return
	}
	if ok, err := m.changerTexte(r, c, n, nouveau); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if !ok {
		m.a.Echec(w, r, http.StatusConflict, "La note vient de changer : rechargez la page.")
		return
	}
	n.Texte = nouveau
	m.repondreCases(w, r, c, n)
}

// nettoyer : retire les cases cochées (ceux qui peuvent modifier la note).
func (m *Module) nettoyer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return
	}
	if !modifiable(c, n) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul son auteur (ou un parent) peut la modifier.")
		return
	}
	nouveau, _ := retirerCochees(n.Texte)
	if ok, err := m.changerTexte(r, c, n, nouveau); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if !ok {
		m.a.Echec(w, r, http.StatusConflict, "La note vient de changer : rechargez la page.")
		return
	}
	n.Texte = nouveau
	m.repondreCases(w, r, c, n)
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
	if ok, err := m.changerTexte(r, c, n, nouveau); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if !ok {
		m.a.Echec(w, r, http.StatusConflict, "La note vient de changer : rechargez la page.")
		return
	}
	n.Texte = nouveau
	m.repondreCases(w, r, c, n)
}
