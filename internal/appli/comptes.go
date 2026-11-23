// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"familycocon/internal/config"
	"familycocon/internal/donnees"
)

// Pages sans session : connexion (adulte ou enfant), création de la
// famille, invitation. Les seuils de la limite de tentatives :
const (
	seuilIP     = 10 // échecs par adresse en 15 min
	seuilCompte = 20 // échecs par compte en 15 min (venus de plusieurs adresses)
	seuilPin    = 5  // échecs de code PIN par enfant en 15 min
	// seuilConfirmation : mauvais mots de passe redemandés (action sensible) par membre en 15 min.
	seuilConfirmation = 5
	// seuilDefis : défis de passkey demandés sans être connecté, par adresse en 15 min.
	seuilDefis = 30
)

func (a *App) routesComptes() {
	a.Mux.HandleFunc("GET /connexion", a.pageConnexion)
	a.Mux.HandleFunc("POST /connexion", a.connexion)
	a.Mux.HandleFunc("GET /connexion/enfant/{id}", a.pageConnexionEnfant)
	a.Mux.HandleFunc("POST /connexion/enfant/{id}", a.connexionEnfant)
	a.Mux.HandleFunc("GET /inscription", a.pageInscription)
	a.Mux.HandleFunc("POST /inscription", a.inscription)
	a.Mux.HandleFunc("GET /invitation/{jeton}", a.pageInvitation)
	a.Mux.HandleFunc("POST /invitation/{jeton}", a.accepterInvitation)
	a.Route("POST /deconnexion", "", Membre, func(w http.ResponseWriter, r *http.Request, c *Contexte) {
		a.FermerSession(w, r)
		a.RedirigerComplet(w, r, "/connexion")
	})
}

// formConnexion : données des pages de connexion et d'inscription.
type formConnexion struct {
	Erreur      string
	Identifiant string
	Suite       string
	Enfants     []donnees.Membre
	Inscription bool
	Famille     string // nom de la famille (invitation)
	Role        string
	Enfant      donnees.Membre
	Prenom      string
	TOTP        bool // second facteur : code de l'application
	Passkeys    bool // second facteur : passkey
	PasskeyOK   bool // connexion par passkey proposée (pas sur une adresse IP)
}

// inscriptionPermise : peut-on créer une nouvelle famille ?
func (a *App) inscriptionPermise(ctx context.Context) bool {
	switch a.Config.Inscriptions {
	case config.InscriptionOuverte:
		return true
	case config.InscriptionPremiere:
		n, err := a.Q.CompterFamilles(ctx)
		return err == nil && n == 0
	}
	return false
}

// suiteSure : adresse de retour après connexion, sur ce site seulement
// (chemin local : pas de « // », de « \ », de caractère de contrôle, que
// les navigateurs retirent ou transforment en « / »).
func suiteSure(s string) string {
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.HasPrefix(s, "/connexion") || len(s) > 500 {
		return "/"
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return "/"
		}
	}
	if u, err := url.Parse(s); err != nil || u.Scheme != "" || u.Host != "" || strings.HasPrefix(u.Path, "//") {
		return "/"
	}
	return s
}

func (a *App) afficherAnonyme(w http.ResponseWriter, r *http.Request, statut int, page, titre string, f formConnexion) {
	v := a.NouvelleVue(w, r, nil, titre, "")
	v.Donnees = f
	if err := a.Gabarits.Executer(w, statut, page, "base.html", v); err != nil {
		a.ErreurInterne(w, r, err)
	}
}

func (a *App) pageConnexion(w http.ResponseWriter, r *http.Request) {
	if c, _ := a.lireSession(w, r); c != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// Plus aucune famille : on commence par en créer une.
	if n, err := a.Q.CompterFamilles(r.Context()); err == nil && n == 0 && a.inscriptionPermise(r.Context()) {
		http.Redirect(w, r, "/inscription", http.StatusSeeOther)
		return
	}
	a.afficherConnexion(w, r, http.StatusOK, formConnexion{Suite: suiteSure(r.URL.Query().Get("suite"))})
}

func (a *App) afficherConnexion(w http.ResponseWriter, r *http.Request, statut int, f formConnexion) {
	if fam, ok := a.AppareilDe(r); ok {
		f.Enfants, _ = a.Q.EnfantsAvecPin(r.Context(), fam)
	}
	f.Inscription = a.inscriptionPermise(r.Context())
	_, _, f.PasskeyOK = a.domaineWebAuthn()
	a.afficherAnonyme(w, r, statut, "comptes/connexion", "Connexion", f)
}

// attenteLisible : « 15 min », « 2 h ».
func attenteLisible(d time.Duration) string {
	if d >= time.Hour {
		return fmt.Sprintf("%d h", int(d.Hours()+0.5))
	}
	return fmt.Sprintf("%d min", max(1, int(d.Minutes()+0.5)))
}

func (a *App) connexion(w http.ResponseWriter, r *http.Request) {
	ident := strings.TrimSpace(r.PostFormValue("identifiant"))
	mdp := r.PostFormValue("mdp")
	f := formConnexion{Identifiant: ident, Suite: suiteSure(r.PostFormValue("suite"))}
	ip := a.IPClient(r)
	if len(ident) > 64 { // aucun identifiant n'est si long (et la clé du limiteur reste petite)
		ident = ident[:64]
	}
	cleCompte := "compte|" + strings.ToLower(ident)

	fin, attente, ok := a.Limiteur.Essayer(time.Now(), Limite{CleIP(ip), seuilIP}, Limite{cleCompte, seuilCompte})
	if !ok {
		a.Journal.Warn("connexion refusée : trop d'essais", "identifiant", ident, "ip", ip.String())
		f.Erreur = "Trop d'essais. Réessayez dans " + attenteLisible(attente) + "."
		a.afficherConnexion(w, r, http.StatusTooManyRequests, f)
		return
	}
	m, err := a.Q.MembreParIdentifiant(r.Context(), sql.NullString{String: ident, Valid: ident != ""})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fin(false)
		a.ErreurInterne(w, r, err)
		return
	}
	empreinte := EmpreinteLeurre // même durée de calcul si le compte n'existe pas
	if err == nil && m.Mdp.Valid {
		empreinte = m.Mdp.String
	}
	bon, occupe := ComparerOccupe(empreinte, mdp)
	if occupe {
		fin(false)
		f.Erreur = "Serveur occupé : réessayez dans un instant."
		a.afficherConnexion(w, r, http.StatusServiceUnavailable, f)
		return
	}
	if err != nil || !m.Mdp.Valid || !bon {
		fin(true)
		a.Journal.Warn("échec de connexion", "identifiant", ident, "ip", ip.String())
		f.Erreur = "Identifiant ou mot de passe incorrect."
		a.afficherConnexion(w, r, http.StatusUnauthorized, f)
		return
	}
	fin(false)
	// Second facteur demandé (application ou passkey) : la connexion attend.
	totp, passkeys, err := a.secondFacteur(r.Context(), m)
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	if totp || passkeys {
		if err := a.debuterSecondFacteur(w, r, m, f.Suite); err != nil {
			f.Erreur = err.Error()
			a.afficherConnexion(w, r, http.StatusServiceUnavailable, f)
		}
		return
	}
	if err := a.terminerConnexion(w, r, m, "mot de passe"); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	http.Redirect(w, r, f.Suite, http.StatusSeeOther)
}

// enfantDeLAppareil : l'enfant {id}, s'il appartient à la famille de cet
// appareil de confiance et a un code PIN.
func (a *App) enfantDeLAppareil(r *http.Request) (donnees.Membre, bool) {
	fam, ok := a.AppareilDe(r)
	if !ok {
		return donnees.Membre{}, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Membre{}, false
	}
	m, err := a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: id, FamilleID: fam})
	// Le code PIN est réservé aux comptes sans mot de passe : il ne doit pas
	// contourner la double authentification d'un ado qui a son propre compte.
	if err != nil || m.Role != RoleEnfant || !m.Pin.Valid || m.Mdp.Valid {
		return donnees.Membre{}, false
	}
	return m, true
}

func (a *App) pageConnexionEnfant(w http.ResponseWriter, r *http.Request) {
	m, ok := a.enfantDeLAppareil(r)
	if !ok {
		http.Redirect(w, r, "/connexion", http.StatusSeeOther)
		return
	}
	a.afficherAnonyme(w, r, http.StatusOK, "comptes/pin", "Code secret", formConnexion{Enfant: m})
}

func (a *App) connexionEnfant(w http.ResponseWriter, r *http.Request) {
	m, ok := a.enfantDeLAppareil(r)
	if !ok {
		http.Redirect(w, r, "/connexion", http.StatusSeeOther)
		return
	}
	f := formConnexion{Enfant: m}
	ip := a.IPClient(r)
	clePin := "pin|" + strconv.FormatInt(m.ID, 10)
	fin, attente, ok := a.Limiteur.Essayer(time.Now(), Limite{CleIP(ip), seuilIP}, Limite{clePin, seuilPin})
	if !ok {
		a.Journal.Warn("code PIN refusé : trop d'essais", "membre", m.ID, "ip", ip.String())
		f.Erreur = "Trop d'essais. Attends " + attenteLisible(attente) + " ou demande à un parent."
		a.afficherAnonyme(w, r, http.StatusTooManyRequests, "comptes/pin", "Code secret", f)
		return
	}
	bon, occupe := ComparerOccupe(m.Pin.String, r.PostFormValue("pin"))
	if occupe {
		fin(false)
		f.Erreur = "Serveur occupé : réessaie dans un instant."
		a.afficherAnonyme(w, r, http.StatusServiceUnavailable, "comptes/pin", "Code secret", f)
		return
	}
	if !bon {
		fin(true)
		a.Journal.Warn("mauvais code PIN", "membre", m.ID, "ip", ip.String())
		f.Erreur = "Ce n'est pas le bon code. Réessaie !"
		a.afficherAnonyme(w, r, http.StatusUnauthorized, "comptes/pin", "Code secret", f)
		return
	}
	fin(false)
	a.Limiteur.Effacer(clePin)
	if err := a.OuvrirSession(r.Context(), w, m); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Info("connexion par code PIN", "membre", m.ID, "ip", ip.String())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// verifierNouveauCompte contrôle prénom, identifiant et mot de passe.
func verifierNouveauCompte(prenom, ident, mdp, mdp2 string) error {
	if err := VerifierPrenom(prenom); err != nil {
		return err
	}
	if err := VerifierIdentifiant(ident); err != nil {
		return err
	}
	if mdp != mdp2 {
		return errors.New("Les deux mots de passe ne sont pas les mêmes.")
	}
	if err := VerifierRegles(mdp); err != nil {
		return err
	}
	return VerifierSimple(mdp, ident)
}

// VerifierPrenom : 1 à 40 caractères, sans caractère de contrôle.
func VerifierPrenom(p string) error {
	n := utf8.RuneCountInString(p)
	if n < 1 || n > 40 {
		return errors.New("Le prénom doit faire entre 1 et 40 caractères.")
	}
	for _, c := range p {
		if unicode.IsControl(c) {
			return errors.New("Le prénom contient un caractère invalide.")
		}
	}
	return nil
}

// VerifierIdentifiant : 3 à 64 caractères parmi lettres, chiffres, . _ - @.
func VerifierIdentifiant(s string) error {
	if len(s) < 3 || len(s) > 64 {
		return errors.New("L'identifiant doit faire entre 3 et 64 caractères.")
	}
	for _, c := range s {
		if !(c < 128 && (unicode.IsLetter(c) || unicode.IsDigit(c) || strings.ContainsRune("._-@", c))) {
			return errors.New("Identifiant : lettres sans accent, chiffres, et . _ - @ seulement.")
		}
	}
	return nil
}

func (a *App) pageInscription(w http.ResponseWriter, r *http.Request) {
	if !a.inscriptionPermise(r.Context()) {
		a.PageErreur(w, r, nil, http.StatusForbidden, "Inscriptions fermées", "La création de famille est fermée sur ce serveur.")
		return
	}
	a.afficherAnonyme(w, r, http.StatusOK, "comptes/inscription", "Créer ma famille", formConnexion{})
}

func (a *App) inscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !a.inscriptionPermise(ctx) {
		a.PageErreur(w, r, nil, http.StatusForbidden, "Inscriptions fermées", "La création de famille est fermée sur ce serveur.")
		return
	}
	nomFamille := strings.TrimSpace(r.PostFormValue("famille"))
	prenom := strings.TrimSpace(r.PostFormValue("prenom"))
	ident := strings.TrimSpace(r.PostFormValue("identifiant"))
	mdp := r.PostFormValue("mdp")
	f := formConnexion{Famille: nomFamille, Prenom: prenom, Identifiant: ident}

	err := verifierNouveauCompte(prenom, ident, mdp, r.PostFormValue("mdp2"))
	if err == nil && (utf8.RuneCountInString(nomFamille) < 1 || utf8.RuneCountInString(nomFamille) > 60) {
		err = errors.New("Le nom de la famille doit faire entre 1 et 60 caractères.")
	}
	if err != nil {
		f.Erreur = err.Error()
		a.afficherAnonyme(w, r, http.StatusBadRequest, "comptes/inscription", "Créer ma famille", f)
		return
	}
	empreinte, err := Empreinte(mdp)
	if err != nil {
		f.Erreur = err.Error()
		a.afficherAnonyme(w, r, http.StatusServiceUnavailable, "comptes/inscription", "Créer ma famille", f)
		return
	}
	var m donnees.Membre
	err = a.Transaction(ctx, func(q *donnees.Queries) error {
		// Vérifié de nouveau dans la transaction : deux inscriptions
		// simultanées ne créent pas deux « premières » familles.
		if a.Config.Inscriptions == config.InscriptionPremiere {
			if n, err := q.CompterFamilles(ctx); err != nil || n > 0 {
				return errors.Join(err, errors.New("inscriptions fermées"))
			}
		}
		fam, err := q.CreerFamille(ctx, donnees.CreerFamilleParams{Nom: nomFamille, Fuseau: "Europe/Paris", Devise: "EUR", Modules: ModulesParDefaut(), CreeLe: time.Now().Unix()})
		if err != nil {
			return err
		}
		m, err = q.CreerMembre(ctx, donnees.CreerMembreParams{
			FamilleID: fam.ID, Nom: prenom, Role: RoleParent, Couleur: 1,
			Identifiant: sql.NullString{String: ident, Valid: true},
			Mdp:         sql.NullString{String: empreinte, Valid: true}, PushModules: PushParDefaut, CreeLe: time.Now().Unix(),
		})
		return err
	})
	if err != nil {
		if estDoublon(err) {
			f.Erreur = "Cet identifiant est déjà pris."
			a.afficherAnonyme(w, r, http.StatusConflict, "comptes/inscription", "Créer ma famille", f)
			return
		}
		a.ErreurInterne(w, r, err)
		return
	}
	if err := a.OuvrirSession(ctx, w, m); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	_ = a.MarquerAppareil(ctx, w, r, m.FamilleID)
	a.Journal.Info("famille créée", "famille", m.FamilleID, "membre", m.ID)
	a.Flash(w, "Bienvenue dans FamilyCocon 🎉")
	http.Redirect(w, r, "/famille", http.StatusSeeOther)
}

// estDoublon : violation d'unicité (identifiant déjà pris).
func estDoublon(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// Transaction exécute f dans une transaction (tout ou rien).
func (a *App) Transaction(ctx context.Context, f func(q *donnees.Queries) error) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // sans effet après Commit
	if err := f(a.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

// invitationValide : l'invitation du lien, si elle existe encore.
func (a *App) invitationValide(r *http.Request) (donnees.InvitationParEmpreinteRow, bool) {
	jeton := r.PathValue("jeton")
	if len(jeton) != 43 {
		return donnees.InvitationParEmpreinteRow{}, false
	}
	inv, err := a.Q.InvitationParEmpreinte(r.Context(), donnees.InvitationParEmpreinteParams{
		Empreinte: Empreinte256(jeton), ExpireLe: time.Now().Unix()})
	return inv, err == nil
}

func (a *App) pageInvitation(w http.ResponseWriter, r *http.Request) {
	inv, ok := a.invitationValide(r)
	if !ok {
		a.PageErreur(w, r, nil, http.StatusNotFound, "Invitation expirée",
			"Ce lien d'invitation n'est plus valable (déjà utilisé, ou plus de 7 jours). Demandez-en un nouveau.")
		return
	}
	a.afficherAnonyme(w, r, http.StatusOK, "comptes/invitation", "Invitation", formConnexion{Famille: inv.FamilleNom, Role: inv.Role})
}

func (a *App) accepterInvitation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inv, ok := a.invitationValide(r)
	if !ok {
		a.PageErreur(w, r, nil, http.StatusNotFound, "Invitation expirée", "Ce lien d'invitation n'est plus valable.")
		return
	}
	prenom := strings.TrimSpace(r.PostFormValue("prenom"))
	ident := strings.TrimSpace(r.PostFormValue("identifiant"))
	mdp := r.PostFormValue("mdp")
	f := formConnexion{Famille: inv.FamilleNom, Role: inv.Role, Prenom: prenom, Identifiant: ident}
	if err := verifierNouveauCompte(prenom, ident, mdp, r.PostFormValue("mdp2")); err != nil {
		f.Erreur = err.Error()
		a.afficherAnonyme(w, r, http.StatusBadRequest, "comptes/invitation", "Invitation", f)
		return
	}
	empreinte, err := Empreinte(mdp)
	if err != nil {
		f.Erreur = err.Error()
		a.afficherAnonyme(w, r, http.StatusServiceUnavailable, "comptes/invitation", "Invitation", f)
		return
	}
	var m donnees.Membre
	err = a.Transaction(ctx, func(q *donnees.Queries) error {
		// L'invitation est supprimée dans la même transaction : usage unique.
		// Deux envois simultanés du même lien : le second ne supprime rien.
		if n, err := q.SupprimerInvitation(ctx, inv.Empreinte); err != nil || n != 1 {
			return errors.Join(err, errInvitationUtilisee)
		}
		couleur, err := couleurLibre(ctx, q, inv.FamilleID)
		if err != nil {
			return err
		}
		m, err = q.CreerMembre(ctx, donnees.CreerMembreParams{
			FamilleID: inv.FamilleID, Nom: prenom, Role: inv.Role, Couleur: couleur,
			Identifiant: sql.NullString{String: ident, Valid: true},
			Mdp:         sql.NullString{String: empreinte, Valid: true}, PushModules: PushParDefaut, CreeLe: time.Now().Unix(),
		})
		return err
	})
	if err != nil {
		if estDoublon(err) {
			f.Erreur = "Cet identifiant est déjà pris."
			a.afficherAnonyme(w, r, http.StatusConflict, "comptes/invitation", "Invitation", f)
			return
		}
		if errors.Is(err, errInvitationUtilisee) {
			a.PageErreur(w, r, nil, http.StatusNotFound, "Invitation expirée", "Ce lien d'invitation a déjà servi.")
			return
		}
		a.ErreurInterne(w, r, err)
		return
	}
	if err := a.OuvrirSession(ctx, w, m); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	if m.Role == RoleParent {
		_ = a.MarquerAppareil(ctx, w, r, m.FamilleID)
	}
	a.Journal.Info("invitation acceptée", "famille", m.FamilleID, "membre", m.ID)
	a.Flash(w, "Bienvenue "+prenom+" 🎉")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// couleurLibre : la première couleur (1 à 8) que personne n'a encore.
func couleurLibre(ctx context.Context, q *donnees.Queries, famille int64) (int64, error) {
	membres, err := q.MembresDeFamille(ctx, famille)
	if err != nil {
		return 0, err
	}
	prises := map[int64]bool{}
	for _, m := range membres {
		prises[m.Couleur] = true
	}
	for c := int64(1); c <= 8; c++ {
		if !prises[c] {
			return c, nil
		}
	}
	return int64(len(membres)%8) + 1, nil
}

// errInvitationUtilisee : le lien a déjà servi (deux envois en même temps).
var errInvitationUtilisee = errors.New("invitation déjà utilisée")
