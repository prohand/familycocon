// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Catégories des notes : celles de la famille (🚗 Voiture, 🏠 Maison…),
// créées, renommées et supprimées par tout le monde. Une note en a une ou
// aucune. Supprimer une catégorie laisse ses notes « sans catégorie ».

// Bornes.
const (
	categoriesMax = 50
	nomCategorie  = 30 // caractères
	emojiMax      = 8  // caractères (un emoji peut en compter plusieurs)
)

// emojisCategories : la grille proposée pour choisir l'emoji d'une
// catégorie (on peut aussi en écrire un autre).
var emojisCategories = []string{
	"🚗", "🏠", "🍳", "🛒", "💶", "🏦", "🧾", "📄",
	"🏥", "💊", "👶", "🎓", "📚", "💼", "💻", "📱",
	"✈️", "🏖️", "🎁", "🎂", "🎄", "🎉", "🐶", "🐱",
	"🌱", "🔧", "🧹", "👕", "📦", "🔑", "💡", "📅",
	"⚽", "🏃", "🎮", "🎵", "🎨", "🎬", "📷", "🍷",
	"☕", "🧸", "❤️", "⭐", "📝", "🔒", "🗂️", "📌",
}

// Categorie : une catégorie montrée (rangée de filtres, formulaire, cartes).
type Categorie struct {
	ID     int64
	Nom    string
	Emoji  string
	Nombre int64 // notes que ce membre y voit
	Active bool  // filtre choisi
}

// Etiquette : « 🚗 Voiture ».
func (c Categorie) Etiquette() string { return strings.TrimSpace(c.Emoji + " " + c.Nom) }

// categories : celles de la famille, dans l'ordre alphabétique.
func (m *Module) categories(ctx context.Context, c *appli.Contexte) ([]Categorie, error) {
	l, err := m.a.Q.CategoriesNotes(ctx, donnees.CategoriesNotesParams{FamilleID: c.Famille.ID, MembreID: sqlNul(c.Membre.ID)})
	if err != nil {
		return nil, err
	}
	out := make([]Categorie, 0, len(l))
	for _, x := range l {
		out = append(out, Categorie{ID: x.ID, Nom: x.Nom, Emoji: x.Emoji, Nombre: x.Nombre})
	}
	return out, nil
}

// lireCategorie : la catégorie choisie dans le formulaire (0 : aucune ;
// une catégorie d'une autre famille compte pour aucune).
func (m *Module) lireCategorie(r *http.Request, c *appli.Contexte) int64 {
	id, err := strconv.ParseInt(r.PostFormValue("categorie"), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	if n, err := m.a.Q.CategorieNoteExiste(r.Context(), donnees.CategorieNoteExisteParams{ID: id, FamilleID: c.Famille.ID}); err != nil || n == 0 {
		return 0
	}
	return id
}

// ranger : la note dans sa catégorie (0 : sans catégorie).
func (m *Module) ranger(ctx context.Context, c *appli.Contexte, note, categorie int64) error {
	v := sqlNul(categorie)
	return m.a.Q.ChangerCategorieNote(ctx, donnees.ChangerCategorieNoteParams{CategorieID: v, ID: note, FamilleID: c.Famille.ID})
}

// vueCategories : la page « Catégories ».
type vueCategories struct {
	Categories []Categorie
	Plein      bool
	Emojis     []string
}

// pageCategories : GET /notes/categories.
func (m *Module) pageCategories(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.categories(r.Context(), c)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "notes/categories", "Catégories des notes", mod, vueCategories{Categories: l, Plein: len(l) >= categoriesMax, Emojis: emojisCategories})
}

// lireNomEmoji : le nom (obligatoire) et l'emoji (facultatif) du formulaire.
func lireNomEmoji(r *http.Request) (nom, emoji, msg string) {
	nom = strings.Join(strings.Fields(r.PostFormValue("nom")), " ")
	emoji = strings.TrimSpace(r.PostFormValue("emoji"))
	switch {
	case nom == "":
		return "", "", "Donnez un nom à la catégorie."
	case utf8.RuneCountInString(nom) > nomCategorie:
		return "", "", fmt.Sprintf("Nom : %d caractères au plus.", nomCategorie)
	case utf8.RuneCountInString(emoji) > emojiMax || strings.ContainsAny(emoji, "<>&\"'"):
		return "", "", "Emoji : un seul, s'il vous plaît."
	}
	return nom, emoji, ""
}

// categoriesPost : POST /notes/categories : creer, renommer (action=modifier)
// ou supprimer (action=supprimer) une catégorie.
func (m *Module) categoriesPost(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	switch r.PostFormValue("action") {
	case "supprimer":
		if err := m.a.Q.SupprimerCategorieNote(ctx, donnees.SupprimerCategorieNoteParams{ID: id, FamilleID: c.Famille.ID}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Flash(w, "Catégorie supprimée (ses notes restent, sans catégorie)")
	case "modifier":
		nom, emoji, msg := lireNomEmoji(r)
		if msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		if err := m.a.Q.ModifierCategorieNote(ctx, donnees.ModifierCategorieNoteParams{Nom: nom, Emoji: emoji, ID: id, FamilleID: c.Famille.ID}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Flash(w, "Catégorie enregistrée ✓")
	default:
		nom, emoji, msg := lireNomEmoji(r)
		if msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		if n, err := m.a.Q.CompterCategoriesNotes(ctx, c.Famille.ID); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		} else if n >= categoriesMax {
			m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("%d catégories au plus.", categoriesMax))
			return
		}
		cat, err := m.a.Q.CreerCategorieNote(ctx, donnees.CreerCategorieNoteParams{FamilleID: c.Famille.ID, Nom: nom, Emoji: emoji, CreeLe: time.Now().Unix()})
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Flash(w, "Catégorie ajoutée ✓")
		// Venue du formulaire d'une note : retour à la liste filtrée sur elle.
		if r.PostFormValue("retour") == "notes" {
			m.a.Publier(c, mod, "")
			m.a.Rediriger(w, r, "/notes?cat="+strconv.FormatInt(cat.ID, 10))
			return
		}
	}
	m.a.Publier(c, mod, "")
	m.a.Rediriger(w, r, "/notes/categories")
}

// sqlNul : un numéro, nul s'il vaut 0.
func sqlNul(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id > 0} }
