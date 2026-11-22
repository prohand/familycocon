// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/donnees"
)

// Réglages de la famille (parents) : nom, fuseau, monnaie, modules activés,
// membres (enfants avec code PIN, invitations des adultes).

// Fuseaux proposés (les plus courants pour une famille francophone).
var Fuseaux = []string{
	"Europe/Paris", "Europe/Brussels", "Europe/Luxembourg", "Europe/Zurich", "Europe/Monaco",
	"Europe/London", "America/Montreal", "America/Guadeloupe", "America/Martinique",
	"America/Cayenne", "America/Miquelon", "Indian/Reunion", "Indian/Mayotte",
	"Pacific/Noumea", "Pacific/Tahiti", "Africa/Casablanca", "Africa/Algiers", "Africa/Tunis",
	"Africa/Dakar", "Africa/Abidjan", "UTC",
}

// Invitations valables 7 jours.
const dureeInvitation = 7 * 24 * time.Hour

type donneesFamille struct {
	Membres     []donnees.Membre
	Fuseaux     []string
	Devises     []string
	Catalogue   []ModuleDef
	Invitations int
	Lien        string // lien d'invitation qui vient d'être créé (affiché une fois)
	Erreur      string
}

func (a *App) routesFamille() {
	a.Route("GET /famille", "", Parent, a.pageFamille)
	a.Route("POST /famille/reglages", "", Parent, a.reglagesFamille)
	a.Route("POST /famille/modules", "", Parent, a.modulesFamille)
	a.Route("POST /famille/enfants", "", Parent, a.ajouterEnfant)
	a.Route("POST /famille/invitations", "", Parent, a.inviter)
	a.Route("POST /famille/invitations/annuler", "", Parent, a.annulerInvitations)
	a.Route("POST /famille/membres/{id}", "", Parent, a.modifierMembre)
	a.Route("POST /famille/membres/{id}/pin", "", Parent, a.changerPin)
	a.Route("POST /famille/membres/{id}/retirer", "", Parent, a.retirerMembre)
}

func (a *App) afficherFamille(w http.ResponseWriter, r *http.Request, c *Contexte, d donneesFamille) {
	var err error
	if d.Membres, err = a.Q.MembresDeFamille(r.Context(), c.Famille.ID); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	inv, _ := a.Q.InvitationsEnCours(r.Context(), donnees.InvitationsEnCoursParams{FamilleID: c.Famille.ID, ExpireLe: time.Now().Unix()})
	d.Invitations = len(inv)
	d.Fuseaux, d.Devises, d.Catalogue = Fuseaux, Devises, Catalogue
	a.Afficher(w, r, c, "famille", "Ma famille", "", d)
}

func (a *App) pageFamille(w http.ResponseWriter, r *http.Request, c *Contexte) {
	a.afficherFamille(w, r, c, donneesFamille{})
}

func (a *App) reglagesFamille(w http.ResponseWriter, r *http.Request, c *Contexte) {
	nom := strings.TrimSpace(r.PostFormValue("nom"))
	fuseau := r.PostFormValue("fuseau")
	devise := r.PostFormValue("devise")
	if n := utf8.RuneCountInString(nom); n < 1 || n > 60 {
		a.Echec(w, r, http.StatusBadRequest, "Le nom de la famille doit faire entre 1 et 60 caractères.")
		return
	}
	if _, err := time.LoadLocation(fuseau); err != nil || !contient(Fuseaux, fuseau) {
		a.Echec(w, r, http.StatusBadRequest, "Fuseau horaire inconnu.")
		return
	}
	if !contient(Devises, devise) {
		a.Echec(w, r, http.StatusBadRequest, "Monnaie inconnue.")
		return
	}
	if err := a.Q.ModifierFamille(r.Context(), donnees.ModifierFamilleParams{Nom: nom, Fuseau: fuseau, Devise: devise, ID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Flash(w, "Réglages enregistrés ✓")
	a.Rediriger(w, r, "/famille")
}

func (a *App) modulesFamille(w http.ResponseWriter, r *http.Request, c *Contexte) {
	if err := r.ParseForm(); err != nil {
		a.Echec(w, r, http.StatusBadRequest, "Formulaire invalide.")
		return
	}
	var actifs []string
	for _, m := range Catalogue {
		if r.PostForm.Get("module_"+m.Cle) == "1" {
			actifs = append(actifs, m.Cle)
		}
	}
	if err := a.Q.ModifierModules(r.Context(), donnees.ModifierModulesParams{Modules: strings.Join(actifs, ","), ID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Info("modules modifiés", "famille", c.Famille.ID, "modules", strings.Join(actifs, ","))
	a.Flash(w, "Modules enregistrés ✓")
	a.Rediriger(w, r, "/famille")
}

func (a *App) ajouterEnfant(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	prenom := strings.TrimSpace(r.PostFormValue("prenom"))
	if err := VerifierPrenom(prenom); err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	pin, err := EmpreintePin(r.PostFormValue("pin"))
	if err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	err = a.Transaction(ctx, func(q *donnees.Queries) error {
		couleur, err := lireCouleur(r.PostFormValue("couleur"))
		if err != nil {
			if couleur, err = couleurLibre(ctx, q, c.Famille.ID); err != nil {
				return err
			}
		}
		enfant, err := q.CreerMembre(ctx, donnees.CreerMembreParams{
			FamilleID: c.Famille.ID, Nom: prenom, Role: RoleEnfant, Couleur: couleur,
			Pin: sql.NullString{String: pin, Valid: true}, PushModules: PushParDefaut, CreeLe: time.Now().Unix(),
		})
		if err != nil {
			return err
		}
		// La barre du bas d'un enfant mène d'abord à son espace.
		return q.ModifierRaccourcis(ctx, donnees.ModifierRaccourcisParams{Raccourcis: RaccourcisEnfant, ID: enfant.ID})
	})
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Flash(w, prenom+" a été ajouté(e) ⭐")
	a.Rediriger(w, r, "/famille")
}

func lireCouleur(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 || n > 8 {
		return 0, errors.New("couleur : 1 à 8")
	}
	return n, nil
}

func (a *App) inviter(w http.ResponseWriter, r *http.Request, c *Contexte) {
	role := r.PostFormValue("role")
	if role != RoleParent && role != RoleEnfant {
		role = RoleParent
	}
	jeton, empreinte := jetonAleatoire()
	t := time.Now()
	if err := a.Q.CreerInvitation(r.Context(), donnees.CreerInvitationParams{
		Empreinte: empreinte, FamilleID: c.Famille.ID, Role: role,
		CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: t.Unix(), ExpireLe: t.Add(dureeInvitation).Unix(),
	}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Info("invitation créée", "famille", c.Famille.ID, "par", c.Membre.ID, "role", role)
	a.afficherFamille(w, r, c, donneesFamille{Lien: a.Config.URLPublique.String() + "/invitation/" + jeton})
}

func (a *App) annulerInvitations(w http.ResponseWriter, r *http.Request, c *Contexte) {
	if err := a.Q.AnnulerInvitations(r.Context(), c.Famille.ID); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Flash(w, "Invitations annulées")
	a.Rediriger(w, r, "/famille")
}

// membreCible : le membre {id} de la famille.
func (a *App) membreCible(r *http.Request, c *Contexte) (donnees.Membre, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Membre{}, sql.ErrNoRows
	}
	return a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: id, FamilleID: c.Famille.ID})
}

// modifierMembre : un parent change le prénom ou la couleur d'un membre.
func (a *App) modifierMembre(w http.ResponseWriter, r *http.Request, c *Contexte) {
	m, err := a.membreCible(r, c)
	if err != nil {
		a.Echec(w, r, http.StatusNotFound, "Membre introuvable.")
		return
	}
	prenom := strings.TrimSpace(r.PostFormValue("prenom"))
	if err := VerifierPrenom(prenom); err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	couleur, err := lireCouleur(r.PostFormValue("couleur"))
	if err != nil {
		couleur = m.Couleur
	}
	naissance, err := lireNaissance(r.PostFormValue("naissance"))
	if err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.Q.ModifierProfil(r.Context(), donnees.ModifierProfilParams{Nom: prenom, Couleur: couleur, ID: m.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	if err := a.Q.ModifierNaissance(r.Context(), donnees.ModifierNaissanceParams{Naissance: naissance, ID: m.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Flash(w, "Enregistré ✓")
	a.Rediriger(w, r, "/famille")
}

func (a *App) changerPin(w http.ResponseWriter, r *http.Request, c *Contexte) {
	m, err := a.membreCible(r, c)
	if err != nil || m.Role != RoleEnfant {
		a.Echec(w, r, http.StatusNotFound, "Enfant introuvable.")
		return
	}
	pin, err := EmpreintePin(r.PostFormValue("pin"))
	if err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.Q.ModifierPin(r.Context(), donnees.ModifierPinParams{Pin: sql.NullString{String: pin, Valid: true}, ID: m.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Limiteur.Effacer("pin|" + strconv.FormatInt(m.ID, 10)) // un nouveau code lève le verrou
	a.Flash(w, "Nouveau code de "+m.Nom+" enregistré ✓")
	a.Rediriger(w, r, "/famille")
}

func (a *App) retirerMembre(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	m, err := a.membreCible(r, c)
	if err != nil {
		a.Echec(w, r, http.StatusNotFound, "Membre introuvable.")
		return
	}
	if m.ID == c.Membre.ID {
		a.Echec(w, r, http.StatusBadRequest, "Vous ne pouvez pas vous retirer vous-même.")
		return
	}
	err = a.Transaction(ctx, func(q *donnees.Queries) error {
		if m.Role == RoleParent {
			if n, err := q.CompterParents(ctx, c.Famille.ID); err != nil || n <= 1 {
				return errors.Join(err, errDernierParent)
			}
		}
		if err := q.RetirerMembre(ctx, donnees.RetirerMembreParams{ID: m.ID, FamilleID: c.Famille.ID}); err != nil {
			return err
		}
		return q.SupprimerSessionsMembre(ctx, m.ID)
	})
	if errors.Is(err, errDernierParent) {
		a.Echec(w, r, http.StatusBadRequest, "Il faut au moins un parent dans la famille.")
		return
	}
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Info("membre retiré", "famille", c.Famille.ID, "membre", m.ID, "par", c.Membre.ID)
	a.Flash(w, m.Nom+" a été retiré(e) de la famille")
	a.Rediriger(w, r, "/famille")
}

var errDernierParent = errors.New("dernier parent")

func contient(liste []string, v string) bool {
	for _, x := range liste {
		if x == v {
			return true
		}
	}
	return false
}

// Membres : membres actifs de la famille (pour les modules).
func (a *App) Membres(ctx context.Context, c *Contexte) ([]donnees.Membre, error) {
	return a.Q.MembresDeFamille(ctx, c.Famille.ID)
}
