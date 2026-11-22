// SPDX-License-Identifier: GPL-3.0-or-later

// Package courses : listes de courses partagées, rangées par rayon, mises à
// jour en temps réel chez toute la famille.
//
// Droits : tout le monde ajoute, coche et corrige un produit ; seuls les
// parents suppriment (produit, produits pris, liste).
package courses

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

// Module : le module Courses.
type Module struct{ a *appli.App }

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	const mod = "courses"
	a.Route("GET /courses", mod, appli.Membre, m.accueil)
	a.Route("POST /courses/listes", mod, appli.Membre, m.creerListe)
	a.Route("GET /courses/liste/{id}", mod, appli.Membre, m.page)
	a.Route("GET /courses/liste/{id}/contenu", mod, appli.Membre, m.contenu)
	a.Route("POST /courses/liste/{id}/produits", mod, appli.Membre, m.ajouter)
	a.Route("POST /courses/liste/{id}/vider", mod, appli.Parent, m.vider)
	a.Route("POST /courses/liste/{id}/renommer", mod, appli.Parent, m.renommer)
	a.Route("POST /courses/liste/{id}/supprimer", mod, appli.Parent, m.supprimerListe)
	a.Route("POST /courses/produit/{pid}/cocher", mod, appli.Membre, m.cocher)
	a.Route("GET /courses/produit/{pid}/modifier", mod, appli.Membre, m.edition)
	a.Route("POST /courses/produit/{pid}", mod, appli.Membre, m.modifier)
	a.Route("POST /courses/produit/{pid}/supprimer", mod, appli.Parent, m.supprimer)

	a.AjouterResume(mod, m.resume)
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "🛒", Texte: "Un produit à acheter", URL: "/courses?ajout=1"})
}

// AjouterProduits : ajoute des produits à la première liste de la famille
// (utilisé par les menus : « ajouter les ingrédients aux courses »).
func AjouterProduits(ctx context.Context, a *appli.App, c *appli.Contexte, saisies []string) (int, error) {
	liste, err := listeParDefaut(ctx, a, c)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, s := range saisies {
		if strings.TrimSpace(s) == "" {
			continue
		}
		if _, err := ajouterProduit(ctx, a, c, liste.ID, s, ""); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// listeParDefaut : la première liste de la famille (créée au besoin).
func listeParDefaut(ctx context.Context, a *appli.App, c *appli.Contexte) (donnees.Liste, error) {
	listes, err := a.Q.ListesDeFamille(ctx, c.Famille.ID)
	if err != nil {
		return donnees.Liste{}, err
	}
	if len(listes) > 0 {
		return listes[0], nil
	}
	return a.Q.CreerListe(ctx, donnees.CreerListeParams{FamilleID: c.Famille.ID, Nom: "Courses", CreeLe: time.Now().Unix()})
}

func (m *Module) accueil(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	liste, err := listeParDefaut(r.Context(), m.a, c)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	cible := fmt.Sprintf("/courses/liste/%d", liste.ID)
	if r.URL.Query().Get("ajout") == "1" {
		cible += "?ajout=1"
	}
	http.Redirect(w, r, cible, http.StatusSeeOther)
}

// liste : la liste {id} de la famille.
func (m *Module) liste(r *http.Request, c *appli.Contexte) (donnees.Liste, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Liste{}, sql.ErrNoRows
	}
	return m.a.Q.ListeParID(r.Context(), donnees.ListeParIDParams{ID: id, FamilleID: c.Famille.ID})
}

// produit : le produit {pid}, s'il appartient à une liste de la famille.
func (m *Module) produit(r *http.Request, c *appli.Contexte) (donnees.Produit, error) {
	id, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		return donnees.Produit{}, sql.ErrNoRows
	}
	return m.a.Q.ProduitParID(r.Context(), donnees.ProduitParIDParams{ID: id, FamilleID: c.Famille.ID})
}

// Données de la page.
type ligneProduit struct {
	donnees.Produit
	Auteur *donnees.Membre
}

type groupeRayon struct {
	Rayon
	Produits    []ligneProduit
	Pris, Total int
}

type vueListe struct {
	Liste       donnees.Liste
	Listes      []donnees.Liste
	Rayons      []groupeRayon
	Pris, Total int
	TousRayons  []Rayon
	Suggestions []string
	Edition     int64 // produit en cours de modification
	Ajout       bool  // venu du bouton « + » : curseur dans le champ
	Rafraichir  bool  // le bloc se recharge sur un changement venu d'ailleurs
}

func (m *Module) vue(ctx context.Context, c *appli.Contexte, liste donnees.Liste) (*vueListe, error) {
	v := &vueListe{Liste: liste, TousRayons: Rayons, Rafraichir: true}
	produits, err := m.a.Q.ProduitsDeListe(ctx, liste.ID)
	if err != nil {
		return nil, err
	}
	membres, err := m.a.Membres(ctx, c)
	if err != nil {
		return nil, err
	}
	parID := map[int64]*donnees.Membre{}
	for i := range membres {
		parID[membres[i].ID] = &membres[i]
	}
	groupes := map[string]*groupeRayon{}
	for _, p := range produits {
		g, ok := groupes[p.Rayon]
		if !ok {
			ray, connu := RayonParCle(p.Rayon)
			if !connu {
				ray, _ = RayonParCle("autre")
			}
			g = &groupeRayon{Rayon: ray}
			groupes[p.Rayon] = g
		}
		l := ligneProduit{Produit: p}
		if p.AjoutePar.Valid {
			l.Auteur = parID[p.AjoutePar.Int64]
		}
		g.Produits = append(g.Produits, l)
		g.Total++
		v.Total++
		if p.Pris == 1 {
			g.Pris++
			v.Pris++
		}
	}
	for _, r := range Rayons { // dans l'ordre du magasin
		if g, ok := groupes[r.Cle]; ok {
			v.Rayons = append(v.Rayons, *g)
			delete(groupes, r.Cle)
		}
	}
	for _, g := range groupes { // rayon inconnu (ne devrait pas arriver)
		v.Rayons = append(v.Rayons, *g)
	}
	return v, nil
}

func (m *Module) page(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	liste, err := m.liste(r, c)
	if err != nil {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Liste introuvable", "Cette liste n'existe plus.")
		return
	}
	v, err := m.vue(ctx, c, liste)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if v.Listes, err = m.a.Q.ListesDeFamille(ctx, c.Famille.ID); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v.Suggestions, _ = m.a.Q.SuggestionsProduits(ctx, c.Famille.ID)
	v.Ajout = r.URL.Query().Get("ajout") == "1"
	m.a.Afficher(w, r, c, "courses/liste", "Courses", "courses", v)
}

// renvoyerContenu : le bloc de la liste (réponse aux actions htmx).
func (m *Module) renvoyerContenu(w http.ResponseWriter, r *http.Request, c *appli.Contexte, liste donnees.Liste, edition int64) {
	v, err := m.vue(r.Context(), c, liste)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v.Edition = edition
	v.Rafraichir = edition == 0 // pas de rechargement pendant une modification
	m.a.Fragment(w, r, c, "courses/liste", "liste", v)
}

func (m *Module) contenu(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	liste, err := m.liste(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Liste introuvable.")
		return
	}
	m.renvoyerContenu(w, r, c, liste, 0)
}

// Saisie limitée : un nom de produit tient en 80 caractères.
const longueurMax = 80

// ajouterProduit : analyse la saisie, choisit le rayon (choisi, retenu par
// la famille, ou deviné), enregistre.
func ajouterProduit(ctx context.Context, a *appli.App, c *appli.Contexte, liste int64, saisie, rayonChoisi string) (donnees.Produit, error) {
	nom, quantite := Analyser(saisie)
	if nom == "" || utf8.RuneCountInString(nom) > longueurMax || utf8.RuneCountInString(quantite) > 20 {
		return donnees.Produit{}, errSaisie
	}
	cle := Cle(nom)
	rayon, emoji := Deviner(nom)
	if connu, err := a.Q.ProduitConnu(ctx, donnees.ProduitConnuParams{FamilleID: c.Famille.ID, Cle: cle}); err == nil {
		rayon, emoji = connu.Rayon, connu.Emoji
	}
	if r, ok := RayonParCle(rayonChoisi); ok && rayonChoisi != rayon {
		rayon = r.Cle
		if emoji == "🛍️" || emoji == "" {
			emoji = r.Emoji
		}
		_ = a.Q.CorrigerRayon(ctx, donnees.CorrigerRayonParams{FamilleID: c.Famille.ID, Cle: cle, Nom: nom, Rayon: rayon, Emoji: emoji, VuLe: time.Now().Unix()})
	}
	p, err := a.Q.AjouterProduit(ctx, donnees.AjouterProduitParams{
		ListeID: liste, Nom: nom, Quantite: quantite, Rayon: rayon, Emoji: emoji,
		AjoutePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
	})
	if err != nil {
		return p, err
	}
	err = a.Q.RetenirProduit(ctx, donnees.RetenirProduitParams{FamilleID: c.Famille.ID, Cle: cle, Nom: nom, Rayon: rayon, Emoji: emoji, VuLe: time.Now().Unix()})
	return p, err
}

var errSaisie = errors.New("saisie invalide")

func (m *Module) ajouter(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	liste, err := m.liste(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Liste introuvable.")
		return
	}
	saisie := strings.TrimSpace(r.PostFormValue("produit"))
	if saisie == "" {
		m.a.Echec(w, r, http.StatusBadRequest, "Écrivez un produit.")
		return
	}
	p, err := ajouterProduit(r.Context(), m.a, c, liste.ID, saisie, r.PostFormValue("rayon"))
	if errors.Is(err, errSaisie) {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Nom de produit trop long (%d caractères au plus).", longueurMax))
		return
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, "courses", fmt.Sprintf("%s a ajouté « %s » %s", c.Membre.Nom, p.Nom, p.Emoji))
	if !appli.EstHtmx(r) {
		http.Redirect(w, r, fmt.Sprintf("/courses/liste/%d", liste.ID), http.StatusSeeOther)
		return
	}
	m.renvoyerContenu(w, r, c, liste, 0)
}

func (m *Module) cocher(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	p, err := m.produit(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Produit introuvable (déjà retiré ?).")
		return
	}
	// L'état voulu est envoyé (pas « inverser ») : deux personnes qui
	// cochent en même temps ne s'annulent pas.
	pris := int64(0)
	parQui := sql.NullInt64{}
	quand := sql.NullInt64{}
	if r.PostFormValue("pris") == "1" {
		pris = 1
		parQui = sql.NullInt64{Int64: c.Membre.ID, Valid: true}
		quand = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
	}
	if err := m.a.Q.CocherProduit(r.Context(), donnees.CocherProduitParams{Pris: pris, PrisPar: parQui, PrisLe: quand, ID: p.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, "courses", "")
	m.renvoyerListeDuProduit(w, r, c, p, 0)
}

func (m *Module) renvoyerListeDuProduit(w http.ResponseWriter, r *http.Request, c *appli.Contexte, p donnees.Produit, edition int64) {
	liste, err := m.a.Q.ListeParID(r.Context(), donnees.ListeParIDParams{ID: p.ListeID, FamilleID: c.Famille.ID})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if !appli.EstHtmx(r) {
		http.Redirect(w, r, fmt.Sprintf("/courses/liste/%d", liste.ID), http.StatusSeeOther)
		return
	}
	m.renvoyerContenu(w, r, c, liste, edition)
}

func (m *Module) edition(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	p, err := m.produit(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Produit introuvable.")
		return
	}
	m.renvoyerListeDuProduit(w, r, c, p, p.ID)
}

func (m *Module) modifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	p, err := m.produit(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Produit introuvable.")
		return
	}
	nom := strings.TrimSpace(r.PostFormValue("nom"))
	quantite := strings.TrimSpace(r.PostFormValue("quantite"))
	if nom == "" || utf8.RuneCountInString(nom) > longueurMax || utf8.RuneCountInString(quantite) > 20 {
		m.a.Echec(w, r, http.StatusBadRequest, "Nom (80 caractères) ou quantité (20) invalide.")
		return
	}
	ray, ok := RayonParCle(r.PostFormValue("rayon"))
	if !ok {
		m.a.Echec(w, r, http.StatusBadRequest, "Rayon inconnu.")
		return
	}
	emoji := p.Emoji
	if ray.Cle != p.Rayon || Cle(nom) != Cle(p.Nom) {
		if r2, e := Deviner(nom); r2 == ray.Cle {
			emoji = e
		} else if emoji == "🛍️" {
			emoji = ray.Emoji
		}
	}
	if err := m.a.Q.ModifierProduit(ctx, donnees.ModifierProduitParams{Nom: nom, Quantite: quantite, Rayon: ray.Cle, Emoji: emoji, ID: p.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	// Le rayon choisi est retenu pour la prochaine fois.
	_ = m.a.Q.CorrigerRayon(ctx, donnees.CorrigerRayonParams{FamilleID: c.Famille.ID, Cle: Cle(nom), Nom: nom, Rayon: ray.Cle, Emoji: emoji, VuLe: time.Now().Unix()})
	m.a.Publier(c, "courses", "")
	m.renvoyerListeDuProduit(w, r, c, p, 0)
}

func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	p, err := m.produit(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Produit introuvable.")
		return
	}
	if err := m.a.Q.SupprimerProduit(r.Context(), p.ID); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, "courses", "")
	m.renvoyerListeDuProduit(w, r, c, p, 0)
}

func (m *Module) vider(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	liste, err := m.liste(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Liste introuvable.")
		return
	}
	n, err := m.a.Q.ViderPris(r.Context(), liste.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if n > 0 {
		m.a.Publier(c, "courses", fmt.Sprintf("%s a rangé les courses 🧺", c.Membre.Nom))
	}
	appli.Notifier(w, appli.Pluriel(n, "produit retiré", "produits retirés"), false)
	m.renvoyerContenu(w, r, c, liste, 0)
}

func nomDeListe(r *http.Request) (string, bool) {
	nom := strings.TrimSpace(r.PostFormValue("nom"))
	n := utf8.RuneCountInString(nom)
	return nom, n >= 1 && n <= 40
}

func (m *Module) creerListe(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	nom, ok := nomDeListe(r)
	if !ok {
		m.a.Echec(w, r, http.StatusBadRequest, "Nom de liste : 1 à 40 caractères.")
		return
	}
	l, err := m.a.Q.CreerListe(r.Context(), donnees.CreerListeParams{FamilleID: c.Famille.ID, Nom: nom, CreeLe: time.Now().Unix()})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Rediriger(w, r, fmt.Sprintf("/courses/liste/%d", l.ID))
}

func (m *Module) renommer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	liste, err := m.liste(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Liste introuvable.")
		return
	}
	nom, ok := nomDeListe(r)
	if !ok {
		m.a.Echec(w, r, http.StatusBadRequest, "Nom de liste : 1 à 40 caractères.")
		return
	}
	if err := m.a.Q.RenommerListe(r.Context(), donnees.RenommerListeParams{Nom: nom, ID: liste.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Rediriger(w, r, fmt.Sprintf("/courses/liste/%d", liste.ID))
}

func (m *Module) supprimerListe(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	liste, err := m.liste(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Liste introuvable.")
		return
	}
	if err := m.a.Q.SupprimerListe(r.Context(), donnees.SupprimerListeParams{ID: liste.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Liste « "+liste.Nom+" » supprimée")
	m.a.Rediriger(w, r, "/courses")
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	n, err := m.a.Q.CompterAPrendre(ctx, c.Famille.ID)
	if err != nil || n == 0 {
		return appli.Resume{Info: "Liste à jour ✓"}
	}
	return appli.Resume{Info: appli.Pluriel(n, "produit à prendre", "produits à prendre"), Badge: int(n)}
}
