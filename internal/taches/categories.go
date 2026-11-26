// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Catégories des tâches (🧹 Ménage, 🍳 Cuisine, 🧺 Linge…) : comme celles
// des notes, mais à part. Tout le monde les crée, les renomme et les
// supprime ; une tâche en a une ou aucune ; supprimer une catégorie laisse
// ses tâches « sans catégorie ». Une catégorie « parents seulement »
// (choisie par un parent) et ses tâches sont cachées aux enfants (sauf à
// qui a créé la tâche ou doit la faire).

// Bornes.
const (
	categoriesMax = 50
	nomCategorie  = 30 // caractères
)

// sansCat : filtre « Sans catégorie » (paramètre « cat »).
const sansCat = "sans"

// emojisCategories : la grille proposée (on peut en écrire un autre).
var emojisCategories = []string{
	"🧹", "🍳", "🧺", "🛁", "🚽", "🛏️", "🧽", "🗑️",
	"🌱", "🌳", "🐶", "🐱", "🛒", "🚗", "🔧", "🛠️",
	"👶", "🎒", "📚", "💊", "💶", "📄", "📦", "♻️",
	"🍽️", "🧊", "🪟", "🏠", "🔑", "💡", "📅", "⭐",
}

// Categorie : une catégorie montrée (filtre, formulaire, lignes).
type Categorie struct {
	ID      int64
	Nom     string
	Emoji   string
	Nombre  int64 // tâches en cours (que ce membre voit)
	Active  bool  // filtre choisi
	Parents bool  // cachée aux enfants
}

// Etiquette : « 🧹 Ménage ».
func (c Categorie) Etiquette() string { return strings.TrimSpace(c.Emoji + " " + c.Nom) }

// categories : celles de la famille, dans l'ordre alphabétique.
func (m *Module) categories(ctx context.Context, c *appli.Contexte) ([]Categorie, error) {
	l, err := m.a.Q.CategoriesTaches(ctx, donnees.CategoriesTachesParams{FamilleID: c.Famille.ID, MembreID: moi(c), Parent: estParent(c)})
	if err != nil {
		return nil, err
	}
	out := make([]Categorie, 0, len(l))
	for _, x := range l {
		out = append(out, Categorie{ID: x.ID, Nom: x.Nom, Emoji: x.Emoji, Nombre: x.Nombre, Parents: x.Parents == 1})
	}
	return out, nil
}

// lireCategorie : la catégorie choisie dans le formulaire (aucune : nulle ;
// une catégorie d'une autre famille, ou des parents pour un enfant, compte
// pour aucune).
func (m *Module) lireCategorie(r *http.Request, c *appli.Contexte) sql.NullInt64 {
	id, err := strconv.ParseInt(r.PostFormValue("categorie"), 10, 64)
	if err != nil || id <= 0 {
		return sql.NullInt64{}
	}
	if n, err := m.a.Q.CategorieTacheExiste(r.Context(), donnees.CategorieTacheExisteParams{ID: id, FamilleID: c.Famille.ID, Parent: estParent(c)}); err != nil || n == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: id, Valid: true}
}

// categorieParents : vrai si la catégorie est réservée aux parents. Si la
// lecture échoue (base occupée…), on la croit réservée : mieux vaut cacher
// à un enfant que montrer ce qui ne devait pas l'être.
func (m *Module) categorieParents(ctx context.Context, cat sql.NullInt64) bool {
	if !cat.Valid {
		return false
	}
	p, err := m.a.Q.CategorieTacheParents(ctx, cat.Int64)
	if errors.Is(err, sql.ErrNoRows) {
		return false // catégorie supprimée
	}
	return err != nil || p == 1
}

// estParent : 1 pour un parent (paramètre des requêtes).
func estParent(c *appli.Contexte) int64 {
	if c.EstParent() {
		return 1
	}
	return 0
}

// moi : le membre connecté (paramètre des requêtes).
func moi(c *appli.Contexte) sql.NullInt64 { return sql.NullInt64{Int64: c.Membre.ID, Valid: true} }

// lireParents : case « Parents seulement » (un parent seulement ; 0 sinon).
func lireParents(r *http.Request, c *appli.Contexte) int64 {
	if c.EstParent() && r.PostFormValue("parents") == "1" {
		return 1
	}
	return 0
}

// lireFiltreCategorie : « cat » de l'adresse (0 : toutes ; -1 : sans catégorie).
func lireFiltreCategorie(v string) int64 {
	if v == sansCat {
		return -1
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// paramCategorie : la valeur de « cat » pour ce filtre (vide : toutes).
func paramCategorie(cat int64) string {
	switch {
	case cat < 0:
		return sansCat
	case cat > 0:
		return strconv.FormatInt(cat, 10)
	}
	return ""
}

// vueCategories : la page « Catégories des tâches ».
type vueCategories struct {
	Categories []Categorie
	Plein      bool
	Emojis     []string
}

// pageCategories : GET /taches/categories.
func (m *Module) pageCategories(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.categories(r.Context(), c)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "taches/categories", "Catégories des tâches", mod,
		vueCategories{Categories: l, Plein: len(l) >= categoriesMax, Emojis: emojisCategories})
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
	case emoji != "" && !appli.EmojiValide(emoji):
		return "", "", "Emoji : un seul, s'il vous plaît."
	}
	return nom, emoji, ""
}

// categoriesPost : POST /taches/categories : créer, renommer
// (action=modifier) ou supprimer (action=supprimer) une catégorie.
func (m *Module) categoriesPost(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	switch r.PostFormValue("action") {
	case "supprimer":
		if err := m.a.Q.SupprimerCategorieTache(ctx, donnees.SupprimerCategorieTacheParams{ID: id, FamilleID: c.Famille.ID, Parent: estParent(c)}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Flash(w, "Catégorie supprimée (ses tâches restent, sans catégorie)")
	case "modifier":
		nom, emoji, msg := lireNomEmoji(r)
		if msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		if err := m.a.Q.ModifierCategorieTache(ctx, donnees.ModifierCategorieTacheParams{Nom: nom, Emoji: emoji, ID: id, FamilleID: c.Famille.ID, Parent: estParent(c)}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Flash(w, "Catégorie enregistrée ✓")
	case "parents":
		// Cacher aux enfants (ou leur montrer) : un parent seulement.
		if !c.EstParent() {
			m.a.Echec(w, r, http.StatusForbidden, "Seul un parent peut changer ce réglage.")
			return
		}
		p := lireParents(r, c)
		if err := m.a.Q.ChangerParentsCategorieTache(ctx, donnees.ChangerParentsCategorieTacheParams{Parents: p, ID: id, FamilleID: c.Famille.ID}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		if p == 1 {
			m.a.Flash(w, "Catégorie cachée aux enfants ✓")
		} else {
			m.a.Flash(w, "Catégorie visible par toute la famille ✓")
		}
	default:
		nom, emoji, msg := lireNomEmoji(r)
		if msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		if n, err := m.a.Q.CompterCategoriesTaches(ctx, c.Famille.ID); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		} else if n >= categoriesMax {
			m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("%d catégories au plus.", categoriesMax))
			return
		}
		if _, err := m.a.Q.CreerCategorieTache(ctx, donnees.CreerCategorieTacheParams{FamilleID: c.Famille.ID, Nom: nom, Emoji: emoji,
			Parents: lireParents(r, c), CreeLe: time.Now().Unix()}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Flash(w, "Catégorie ajoutée ✓")
	}
	m.a.Publier(c, mod, "")
	m.a.Rediriger(w, r, "/taches/categories")
}
