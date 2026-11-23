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

// Plafonds par famille : assez pour toute une vie de courses, et la base
// ne peut pas être remplie par jeu.
const (
	listesMax         = 50
	produitsMax       = 2000 // par liste
	produitsConnusMax = 5000 // noms retenus pour les suggestions
)

// SaisieValide : la ligne peut-elle devenir un produit (nom de 80
// caractères au plus, quantité de 20 au plus) ? Les menus s'en servent pour
// refuser un ingrédient trop long dès l'enregistrement du repas.
func SaisieValide(saisie string) bool {
	nom, quantite := Analyser(saisie)
	return nom != "" && utf8.RuneCountInString(nom) <= longueurMax && utf8.RuneCountInString(quantite) <= quantiteMax
}

// AjouterProduits : ajoute des produits à la première liste de la famille
// (utilisé par les menus : « ajouter les ingrédients aux courses »).
// q : les requêtes de la transaction de l'appelant (tout ou rien). Une
// ligne qui ne peut pas devenir un produit (trop longue) est ignorée, et
// l'ajout s'arrête sans erreur quand la liste est pleine.
func AjouterProduits(ctx context.Context, q *donnees.Queries, c *appli.Contexte, saisies []string) (int, error) {
	liste, err := listeParDefaut(ctx, q, c)
	if err != nil {
		return 0, err
	}
	deja, err := q.CompterProduitsListe(ctx, liste.ID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, s := range saisies {
		if !SaisieValide(s) {
			continue
		}
		if deja+int64(n) >= produitsMax {
			break
		}
		if _, err := ajouterProduit(ctx, q, c, liste.ID, s, ""); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// listeParDefaut : la première liste de la famille (créée au besoin).
// À appeler dans une transaction : deux ouvertures en même temps ne créent
// pas deux listes « Courses ».
func listeParDefaut(ctx context.Context, q *donnees.Queries, c *appli.Contexte) (donnees.Liste, error) {
	listes, err := q.ListesDeFamille(ctx, c.Famille.ID)
	if err != nil {
		return donnees.Liste{}, err
	}
	if len(listes) > 0 {
		return listes[0], nil
	}
	return q.CreerListe(ctx, donnees.CreerListeParams{FamilleID: c.Famille.ID, Nom: "Courses", CreeLe: time.Now().Unix()})
}

func (m *Module) accueil(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	var liste donnees.Liste
	err := m.a.Transaction(r.Context(), func(q *donnees.Queries) (err error) {
		liste, err = listeParDefaut(r.Context(), q, c)
		return err
	})
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
	parID := appli.MembresParID(membres)
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
	if v.Suggestions, err = m.a.Q.SuggestionsProduits(ctx, c.Famille.ID); err != nil {
		m.a.Journal.Warn("courses : suggestions illisibles", "erreur", err) // la page marche sans
	}
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

// Saisie limitée : un nom de produit tient en 80 caractères, une quantité en 20.
const (
	longueurMax = 80
	quantiteMax = 20
)

// peutRetenir : le produit est-il déjà connu de la famille, ou reste-t-il
// de la place pour le retenir (suggestions, rayon appris) ?
func peutRetenir(ctx context.Context, q *donnees.Queries, famille int64, connu bool) bool {
	if connu {
		return true
	}
	n, err := q.CompterProduitsConnus(ctx, famille)
	return err == nil && n < produitsConnusMax
}

// ajouterProduit : analyse la saisie, choisit le rayon (choisi, retenu par
// la famille, ou deviné), enregistre. q : a.Q, ou les requêtes d'une transaction.
func ajouterProduit(ctx context.Context, q *donnees.Queries, c *appli.Contexte, liste int64, saisie, rayonChoisi string) (donnees.Produit, error) {
	if !SaisieValide(saisie) {
		return donnees.Produit{}, errSaisie
	}
	nom, quantite := Analyser(saisie)
	cle := Cle(nom)
	rayon, emoji := Deviner(nom)
	connu, err := q.ProduitConnu(ctx, donnees.ProduitConnuParams{FamilleID: c.Famille.ID, Cle: cle})
	dejaConnu := err == nil
	if dejaConnu {
		rayon, emoji = connu.Rayon, connu.Emoji
	}
	retenir := peutRetenir(ctx, q, c.Famille.ID, dejaConnu)
	if r, ok := RayonParCle(rayonChoisi); ok && rayonChoisi != rayon {
		rayon = r.Cle
		if emoji == "🛍️" || emoji == "" {
			emoji = r.Emoji
		}
		if retenir {
			if err := q.CorrigerRayon(ctx, donnees.CorrigerRayonParams{FamilleID: c.Famille.ID, Cle: cle, Nom: nom, Rayon: rayon, Emoji: emoji, VuLe: time.Now().Unix()}); err != nil {
				return donnees.Produit{}, err
			}
		}
	}
	p, err := q.AjouterProduit(ctx, donnees.AjouterProduitParams{
		ListeID: liste, Nom: nom, Quantite: quantite, Rayon: rayon, Emoji: emoji,
		AjoutePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
	})
	if err != nil || !retenir {
		return p, err
	}
	err = q.RetenirProduit(ctx, donnees.RetenirProduitParams{FamilleID: c.Famille.ID, Cle: cle, Nom: nom, Rayon: rayon, Emoji: emoji, VuLe: time.Now().Unix()})
	return p, err
}

var (
	errSaisie      = errors.New("saisie invalide")
	errListePleine = errors.New("liste pleine")
)

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
	var p donnees.Produit
	err = m.a.Transaction(r.Context(), func(q *donnees.Queries) error {
		n, err := q.CompterProduitsListe(r.Context(), liste.ID)
		if err != nil {
			return err
		}
		if n >= produitsMax {
			return errListePleine
		}
		p, err = ajouterProduit(r.Context(), q, c, liste.ID, saisie, r.PostFormValue("rayon"))
		return err
	})
	if errors.Is(err, errSaisie) {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Nom du produit (%d caractères au plus) ou quantité (%d) trop long.", longueurMax, quantiteMax))
		return
	}
	if errors.Is(err, errListePleine) {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Cette liste est pleine (%d produits) : rangez les produits pris.", produitsMax))
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
	if nom == "" || utf8.RuneCountInString(nom) > longueurMax || utf8.RuneCountInString(quantite) > quantiteMax {
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
	// Le rayon choisi est retenu pour la prochaine fois (s'il reste de la place).
	_, errConnu := m.a.Q.ProduitConnu(ctx, donnees.ProduitConnuParams{FamilleID: c.Famille.ID, Cle: Cle(nom)})
	if peutRetenir(ctx, m.a.Q, c.Famille.ID, errConnu == nil) {
		if err := m.a.Q.CorrigerRayon(ctx, donnees.CorrigerRayonParams{FamilleID: c.Famille.ID, Cle: Cle(nom), Nom: nom, Rayon: ray.Cle, Emoji: emoji, VuLe: time.Now().Unix()}); err != nil {
			m.a.Journal.Warn("courses : rayon non retenu", "erreur", err) // le produit, lui, est bien modifié
		}
	}
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
	if !appli.EstHtmx(r) { // comme les autres actions : jamais un bloc nu sans htmx
		http.Redirect(w, r, fmt.Sprintf("/courses/liste/%d", liste.ID), http.StatusSeeOther)
		return
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
	var l donnees.Liste
	err := m.a.Transaction(r.Context(), func(q *donnees.Queries) error {
		n, err := q.CompterListes(r.Context(), c.Famille.ID)
		if err != nil {
			return err
		}
		if n >= listesMax {
			return errListePleine
		}
		l, err = q.CreerListe(r.Context(), donnees.CreerListeParams{FamilleID: c.Famille.ID, Nom: nom, CreeLe: time.Now().Unix()})
		return err
	})
	if errors.Is(err, errListePleine) {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("%d listes au plus : demandez à un parent d'en supprimer une.", listesMax))
		return
	}
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
