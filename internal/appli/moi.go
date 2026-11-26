// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

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
	// Résumés des parties repliées.
	BarreTexte string // « Accueil, Agenda, Courses, Moi »
	NbPush     int    // notifications choisies
	NbPushMax  int
}

func (a *App) routesMoi() {
	a.Route("GET /moi", "", Membre, a.pageMoi)
	a.Route("POST /moi/profil", "", Membre, a.profilMoi)
	a.Route("POST /moi/raccourcis", "", Membre, a.raccourcisMoi)
	a.Route("POST /moi/theme", "", Membre, a.themeMoi)
	a.Route("POST /moi/mdp", "", Membre, a.mdpMoi)
	a.Route("GET /moi/accueil", "", Membre, a.pageMonAccueil)
	a.Route("POST /moi/accueil", "", Membre, a.monAccueil)
	a.Route("POST /moi/accueil/replie", "", Membre, a.blocReplie)
}

func (a *App) pageMoi(w http.ResponseWriter, r *http.Request, c *Contexte) {
	d := donneesMoi{Possibles: RaccourcisPossibles(c), Couleurs: []int{1, 2, 3, 4, 5, 6, 7, 8}, AMotDePasse: c.Membre.Mdp.Valid,
		Push: strings.Split(c.Membre.PushModules, ",")}
	for _, m := range ModulesVisibles(c) {
		if m.Notifie {
			d.Modules = append(d.Modules, m)
		}
	}
	d.Appareils, _ = a.Q.CompterAbonnements(r.Context(), c.Membre.ID)
	d.TOTP = c.Membre.Totp != ""
	d.NbPasskeys, _ = a.Q.CompterPasskeys(r.Context(), c.Membre.ID)
	var noms []string
	for _, rc := range Raccourcis(c) {
		d.Choisis = append(d.Choisis, rc.Cle)
		noms = append(noms, rc.Nom)
	}
	d.BarreTexte = strings.Join(noms, ", ")
	d.NbPushMax = len(d.Modules) + 1 // + les rappels
	for _, cle := range append([]string{"rappels"}, cleModules(d.Modules)...) {
		if slices.Contains(d.Push, cle) {
			d.NbPush++
		}
	}
	a.Afficher(w, r, c, "moi", "Moi", "", d)
}

// cleModules : les clés des modules.
func cleModules(l []ModuleDef) []string {
	var out []string
	for _, m := range l {
		out = append(out, m.Cle)
	}
	return out
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
	naissance, err := lireNaissance(r.PostFormValue("naissance"))
	if err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	// Champ absent (ancienne page) ou hors de la liste : l'avatar ne change pas.
	avatar := r.PostFormValue("avatar")
	if !r.PostForm.Has("avatar") || !AvatarValide(avatar) {
		avatar = c.Membre.Avatar
	}
	if err := a.Q.ModifierProfil(r.Context(), donnees.ModifierProfilParams{Nom: prenom, Couleur: couleur, Avatar: avatar, ID: c.Membre.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	if err := a.Q.ModifierNaissance(r.Context(), donnees.ModifierNaissanceParams{Naissance: naissance, ID: c.Membre.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Flash(w, "Profil enregistré ✓")
	a.Rediriger(w, r, "/moi")
}

// themeMoi : clair, sombre ou automatique, gardé par cet appareil (un
// téléphone partagé peut avoir un autre réglage qu'un ordinateur).
func (a *App) themeMoi(w http.ResponseWriter, r *http.Request, c *Contexte) {
	t := r.PostFormValue("theme")
	if t != "" && !slices.Contains(Themes, t) {
		a.Echec(w, r, http.StatusBadRequest, "Thème inconnu.")
		return
	}
	a.PoserPreference(w, "theme", t)
	a.Flash(w, "Thème enregistré ✓")
	a.RedirigerComplet(w, r, "/moi") // toute la page change de couleurs
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

// pageMonAccueil : l'ordre des modules de l'accueil et ceux qui sont masqués.
func (a *App) pageMonAccueil(w http.ResponseWriter, r *http.Request, c *Contexte) {
	a.Afficher(w, r, c, "mon-accueil", "Mon accueil", "", vueMonAccueil{Modules: ModulesAccueil(c),
		JourHaut: c.Membre.AccueilJourHaut == 1, Compact: c.Membre.AccueilCompact == 1})
}

// BlocsRepliables : les blocs de l'accueil qu'on peut replier.
var BlocsRepliables = []string{"a-venir", "agenda", "taches"}

// blocReplie : POST /moi/accueil/replie (« bloc », « replie » = 1 ou 0),
// envoyé par app.js quand on replie ou déplie un bloc de l'accueil.
func (a *App) blocReplie(w http.ResponseWriter, r *http.Request, c *Contexte) {
	bloc := r.PostFormValue("bloc")
	if !slices.Contains(BlocsRepliables, bloc) {
		a.Echec(w, r, http.StatusBadRequest, "Bloc inconnu.")
		return
	}
	var l []string
	for _, b := range BlocsRepliables {
		if (b == bloc && r.PostFormValue("replie") == "1") || (b != bloc && c.Replie(b)) {
			l = append(l, b)
		}
	}
	if err := a.Q.ModifierBlocsReplies(r.Context(), donnees.ModifierBlocsRepliesParams{AccueilReplies: strings.Join(l, ","),
		ID: c.Membre.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// vueMonAccueil : les modules dans l'ordre du membre et ses deux réglages.
type vueMonAccueil struct {
	Modules           []ModuleAccueil
	JourHaut, Compact bool
}

// monAccueil : POST /moi/accueil. « ordre » : les modules dans l'ordre
// affiché ; « affiche_<module> » coché : la tuile est montrée ; « action » :
// « monter:courses », « descendre:courses », « defaut » ou rien
// (enregistrer). Chaque flèche enregistre aussi les cases.
func (a *App) monAccueil(w http.ResponseWriter, r *http.Request, c *Contexte) {
	permis := map[string]bool{}
	for _, m := range ModulesVisibles(c) {
		permis[m.Cle] = true
	}
	var ordre []string
	vus := map[string]bool{}
	for _, cle := range strings.Split(r.PostFormValue("ordre"), ",") {
		if permis[cle] && !vus[cle] {
			vus[cle] = true
			ordre = append(ordre, cle)
		}
	}
	action, cible, _ := strings.Cut(r.PostFormValue("action"), ":")
	if i := slices.Index(ordre, cible); i >= 0 {
		switch {
		case action == "monter" && i > 0:
			ordre[i-1], ordre[i] = ordre[i], ordre[i-1]
		case action == "descendre" && i < len(ordre)-1:
			ordre[i+1], ordre[i] = ordre[i], ordre[i+1]
		}
	}
	valeur := ""
	if action != "defaut" {
		var l []string
		for _, cle := range ordre {
			if r.PostFormValue("affiche_"+cle) != "1" {
				cle = "-" + cle
			}
			l = append(l, cle)
		}
		valeur = strings.Join(l, ",")
	}
	if err := a.Q.ModifierAccueil(r.Context(), donnees.ModifierAccueilParams{Accueil: valeur, ID: c.Membre.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	// Les deux réglages (cases du même formulaire ; « Remettre l'ordre de
	// départ » ne touche qu'à l'ordre).
	if action != "defaut" {
		var haut, compact int64
		if r.PostFormValue("jour_haut") == "1" {
			haut = 1
		}
		if r.PostFormValue("compact") == "1" {
			compact = 1
		}
		if err := a.Q.ModifierAccueilReglages(r.Context(), donnees.ModifierAccueilReglagesParams{AccueilJourHaut: haut, AccueilCompact: compact,
			ID: c.Membre.ID, FamilleID: c.Famille.ID}); err != nil {
			a.ErreurInterne(w, r, err)
			return
		}
	}
	if action == "" || action == "defaut" {
		a.Flash(w, "Accueil enregistré ✓")
	}
	a.Rediriger(w, r, "/moi/accueil")
}

func (a *App) mdpMoi(w http.ResponseWriter, r *http.Request, c *Contexte) {
	if !c.Membre.Mdp.Valid {
		a.Echec(w, r, http.StatusBadRequest, "Ce compte se connecte avec un code PIN.")
		return
	}
	// Mot de passe actuel : essais limités comme pour toute action sensible
	// (une session volée ne permet pas de le deviner en ligne).
	if statut, msg := a.verifierMotDePasse(r, c, r.PostFormValue("actuel")); msg != "" {
		a.Echec(w, r, statut, msg)
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
	if err := VerifierRegles(nouveau); err != nil {
		a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	empreinte, err := Empreinte(nouveau)
	if err != nil { // serveur occupé (trop de calculs en même temps)
		a.Echec(w, r, http.StatusServiceUnavailable, "Serveur occupé : réessayez dans un instant.")
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

// lireNaissance : date de naissance d'un membre (« » si vide).
func lireNaissance(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.After(time.Now()) || t.Year() < 1900 {
		return "", errors.New("Date de naissance invalide.")
	}
	return s, nil
}
