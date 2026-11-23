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
	"familycocon/internal/photo"
)

// Réglages de la famille (parents) : nom, fuseau, monnaie, modules activés,
// membres (enfants avec code PIN, invitations des adultes).

// ModuleFamille : message temps réel « les réglages de la famille ont changé ».
const ModuleFamille = "famille"

// Fuseaux proposés (les plus courants pour une famille francophone, et ceux
// des pays voisins).
var Fuseaux = []string{
	"Europe/Paris", "Europe/Brussels", "Europe/Luxembourg", "Europe/Zurich", "Europe/Monaco",
	"Europe/London", "Europe/Dublin", "Europe/Lisbon", "Europe/Madrid", "Europe/Berlin", "Europe/Amsterdam",
	"Europe/Rome", "Europe/Andorra", "America/Montreal", "America/Toronto", "America/New_York", "America/Guadeloupe", "America/Martinique",
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
	Serveur     *carteServeur // serveur d'une seule famille : état du serveur
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
	a.Route("POST /famille/appareils/oublier", "", Parent, a.oublierAppareils)
	a.Route("GET /famille/export.json", "", Parent, a.exporter)
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
	d.Serveur = a.etatServeur(r.Context(), c)
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
	// Les autres membres connectés rechargent leur page : tuiles, barre du
	// bas et pages changent avec les modules.
	a.Publier(c, ModuleFamille, "")
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
	// Champ absent (ancienne page) ou hors de la liste : l'avatar ne change pas.
	avatar := r.PostFormValue("avatar")
	if !r.PostForm.Has("avatar") || !AvatarValide(avatar) {
		avatar = m.Avatar
	}
	if err := a.Q.ModifierProfil(r.Context(), donnees.ModifierProfilParams{Nom: prenom, Couleur: couleur, Avatar: avatar, ID: m.ID, FamilleID: c.Famille.ID}); err != nil {
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
	if m.Mdp.Valid {
		a.Echec(w, r, http.StatusBadRequest, m.Nom+" a son propre compte avec mot de passe : pas de code PIN.")
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
	// Les photos de ses notes privées, effacées avec elles (lues avant la
	// transaction : une seule connexion à la base).
	photosPrivees, err := a.Q.PhotosNotesPriveesMembre(ctx, donnees.PhotosNotesPriveesMembreParams{FamilleID: c.Famille.ID,
		AuteurID: sql.NullInt64{Int64: m.ID, Valid: true}})
	if err != nil {
		a.ErreurInterne(w, r, err)
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
		if err := q.SupprimerAbonnementsPushMembre(ctx, m.ID); err != nil {
			return err
		}
		// Le membre reste en base (désactivé) : les « ON DELETE » ne jouent pas.
		// Ses tâches reviennent à tout le monde ; ses notes privées, que
		// personne d'autre ne peut voir ni supprimer, sont effacées.
		moi := sql.NullInt64{Int64: m.ID, Valid: true}
		if err := q.LibererTachesMembre(ctx, donnees.LibererTachesMembreParams{FamilleID: c.Famille.ID, AssigneA: moi}); err != nil {
			return err
		}
		if err := q.SupprimerNotesPriveesMembre(ctx, donnees.SupprimerNotesPriveesMembreParams{FamilleID: c.Famille.ID, AuteurID: moi}); err != nil {
			return err
		}
		// Son budget perso (opérations, celles qui reviennent, plafonds) :
		// personne d'autre ne le voit. Les virements déjà reçus au compte
		// commun y restent.
		if err := q.SupprimerBudgetPerso(ctx, donnees.SupprimerBudgetPersoParams{FamilleID: c.Famille.ID, PersoDe: m.ID}); err != nil {
			return err
		}
		if err := q.SupprimerRecurrencesPerso(ctx, donnees.SupprimerRecurrencesPersoParams{FamilleID: c.Famille.ID, PersoDe: m.ID}); err != nil {
			return err
		}
		if err := q.SupprimerPlafondsPerso(ctx, donnees.SupprimerPlafondsPersoParams{FamilleID: c.Famille.ID, PersoDe: m.ID}); err != nil {
			return err
		}
		// Ce que les modules gardaient privé pour lui (projets privés…).
		for _, f := range a.departs {
			if err := f(ctx, q, c.Famille.ID, m.ID); err != nil {
				return err
			}
		}
		// Plus rien pour se connecter : ni passkey ni code de secours.
		if err := q.SupprimerPasskeysMembre(ctx, m.ID); err != nil {
			return err
		}
		if err := q.EffacerCodesSecours(ctx, m.ID); err != nil {
			return err
		}
		// Son lien secret vers l'agenda ne marche plus.
		if err := q.SupprimerIcal(ctx, m.ID); err != nil {
			return err
		}
		// Un parent retiré : ses appareils de confiance ne doivent plus
		// proposer les enfants. On les oublie tous (les autres parents les
		// retrouvent à leur prochaine connexion).
		if m.Role == RoleParent {
			if err := q.SupprimerAppareilsFamille(ctx, c.Famille.ID); err != nil {
				return err
			}
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
	for _, p := range photosPrivees {
		if err := photo.Effacer(a.DossierPieces(), c.Famille.ID, p.Fichier); err != nil {
			a.Journal.Warn("photo de note non effacée", "fichier", p.Fichier, "erreur", err)
		}
	}
	a.Journal.Info("membre retiré", "famille", c.Famille.ID, "membre", m.ID, "par", c.Membre.ID)
	a.Direct.FermerMembre(c.Famille.ID, m.ID)
	if m.Role == RoleParent {
		_ = a.MarquerAppareil(ctx, w, r, c.Famille.ID) // cet appareil reste de confiance
	}
	a.Flash(w, m.Nom+" a été retiré(e) de la famille")
	a.Rediriger(w, r, "/famille")
}

// oublierAppareils : plus aucun appareil de confiance (tablette perdue,
// téléphone vendu) ; celui-ci le redevient aussitôt.
func (a *App) oublierAppareils(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	if err := a.Q.SupprimerAppareilsFamille(ctx, c.Famille.ID); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	_ = a.MarquerAppareil(ctx, w, r, c.Famille.ID)
	a.Journal.Info("appareils de confiance oubliés", "famille", c.Famille.ID, "par", c.Membre.ID)
	a.Flash(w, "Appareils de confiance oubliés ✓ (sauf celui-ci)")
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

// Membres : membres actifs de la famille (pour les modules). Lus une seule
// fois par requête : un Contexte ne vit que le temps d'une requête. La
// liste est partagée : ne pas la modifier.
func (a *App) Membres(ctx context.Context, c *Contexte) ([]donnees.Membre, error) {
	if c.membresLu {
		return c.membres, nil
	}
	l, err := a.Q.MembresDeFamille(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	c.membres, c.membresLu = l, true
	return l, nil
}
