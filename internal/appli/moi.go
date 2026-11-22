// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"database/sql"
	"net/http"
	"strings"

	"familycocon/internal/donnees"
)

// Page « Moi » : prénom, couleur, raccourcis de la barre du bas, mot de
// passe, déconnexion.

type donneesMoi struct {
	Possibles   []Raccourci
	Choisis     []string // les 4 cases actuelles
	Couleurs    []int
	AMotDePasse bool
	Modules     []ModuleDef // modules qui peuvent notifier
	Push        []string    // modules choisis
	Appareils   int64       // appareils abonnés
	TOTP        bool        // double authentification active
	NbPasskeys  int64
}

func (a *App) routesMoi() {
	a.Route("GET /moi", "", Membre, a.pageMoi)
	a.Route("POST /moi/profil", "", Membre, a.profilMoi)
	a.Route("POST /moi/raccourcis", "", Membre, a.raccourcisMoi)
	a.Route("POST /moi/mdp", "", Membre, a.mdpMoi)
}

func (a *App) pageMoi(w http.ResponseWriter, r *http.Request, c *Contexte) {
	d := donneesMoi{Possibles: RaccourcisPossibles(c), Couleurs: []int{1, 2, 3, 4, 5, 6, 7, 8}, AMotDePasse: c.Membre.Mdp.Valid,
		Modules: ModulesVisibles(c), Push: strings.Split(c.Membre.PushModules, ",")}
	d.Appareils, _ = a.Q.CompterAbonnements(r.Context(), c.Membre.ID)
	d.TOTP = c.Membre.Totp != ""
	d.NbPasskeys, _ = a.Q.CompterPasskeys(r.Context(), c.Membre.ID)
	for _, rc := range Raccourcis(c) {
		d.Choisis = append(d.Choisis, rc.Cle)
	}
	a.Afficher(w, r, c, "moi", "Moi", "", d)
}

func (a *App) profilMoi(w http.ResponseWriter, r *http.Request, c *Contexte) {
	prenom := strings.TrimSpace(r.PostFormValue("prenom"))
	if err := VerifierPrenom(prenom); err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	couleur, err := lireCouleur(r.PostFormValue("couleur"))
	if err != nil {
		couleur = c.Membre.Couleur
	}
	if err := a.Q.ModifierProfil(r.Context(), donnees.ModifierProfilParams{Nom: prenom, Couleur: couleur, ID: c.Membre.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Flash(w, "Profil enregistré ✓")
	a.Rediriger(w, r, "/moi")
}

func (a *App) raccourcisMoi(w http.ResponseWriter, r *http.Request, c *Contexte) {
	possibles := map[string]bool{}
	for _, p := range RaccourcisPossibles(c) {
		possibles[p.Cle] = true
	}
	var choix []string
	vus := map[string]bool{}
	for _, champ := range []string{"r1", "r2", "r3", "r4"} {
		v := r.PostFormValue(champ)
		if !possibles[v] || vus[v] {
			a.Echec(w, r, http.StatusBadRequest, "Choisissez 4 raccourcis différents.")
			return
		}
		vus[v] = true
		choix = append(choix, v)
	}
	if err := a.Q.ModifierRaccourcis(r.Context(), donnees.ModifierRaccourcisParams{Raccourcis: strings.Join(choix, ","), ID: c.Membre.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Flash(w, "Barre du bas enregistrée ✓")
	a.Rediriger(w, r, "/moi")
}

func (a *App) mdpMoi(w http.ResponseWriter, r *http.Request, c *Contexte) {
	if !c.Membre.Mdp.Valid {
		a.Echec(w, r, http.StatusBadRequest, "Ce compte se connecte avec un code PIN.")
		return
	}
	bon, occupe := ComparerOccupe(c.Membre.Mdp.String, r.PostFormValue("actuel"))
	if occupe {
		a.Echec(w, r, http.StatusServiceUnavailable, "Serveur occupé : réessayez dans un instant.")
		return
	}
	if !bon {
		a.Journal.Warn("changement de mot de passe : mot de passe actuel faux", "membre", c.Membre.ID, "ip", a.IPClient(r).String())
		a.Echec(w, r, http.StatusBadRequest, "Mot de passe actuel incorrect.")
		return
	}
	nouveau := r.PostFormValue("nouveau")
	if nouveau != r.PostFormValue("nouveau2") {
		a.Echec(w, r, http.StatusBadRequest, "Les deux mots de passe ne sont pas les mêmes.")
		return
	}
	if err := VerifierSimple(nouveau, c.Membre.Identifiant.String); err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	empreinte, err := Empreinte(nouveau)
	if err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.Q.ModifierMdp(r.Context(), donnees.ModifierMdpParams{Mdp: sql.NullString{String: empreinte, Valid: true}, ID: c.Membre.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	// Les autres appareils sont déconnectés ; celui-ci reste connecté.
	if err := a.Q.SupprimerAutresSessions(r.Context(), donnees.SupprimerAutresSessionsParams{MembreID: c.Membre.ID, Empreinte: c.jeton}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Info("mot de passe changé", "membre", c.Membre.ID)
	a.Flash(w, "Mot de passe changé ✓ (autres appareils déconnectés)")
	a.Rediriger(w, r, "/moi")
}
