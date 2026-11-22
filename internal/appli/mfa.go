// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/mfa"
)

// Double authentification et passkeys, pour les comptes à mot de passe
// (parents, ados). Les enfants à code PIN n'en ont pas : ils se connectent
// sur un appareil de confiance.
//
// Connexion :
//   - mot de passe, puis (si activé) un second facteur : code de
//     l'application (TOTP), code de secours, ou passkey ;
//   - ou une passkey seule, sans mot de passe : l'appareil doit alors
//     vérifier la personne (code, empreinte, visage), ce qui vaut deux facteurs.
//
// Entre le mot de passe et le second facteur, la connexion « en attente »
// vit 5 minutes en mémoire (cookie fc_attente).

const (
	dureeAttente = 5 * time.Minute
	seuilMFA     = 5 // codes faux par compte en 15 min
)

// attenteConnexion : mot de passe vérifié, second facteur attendu.
type attenteConnexion struct {
	Membre int64
	Suite  string
	Defi   string // défi de passkey en cours
}

// domaineWebAuthn : domaine (rpId) et origine, tirés de FC_URL_PUBLIQUE.
// Faux pour une adresse IP (les navigateurs refusent les passkeys).
func (a *App) domaineWebAuthn() (domaine, origine string, possible bool) {
	u := a.Config.URLPublique
	domaine = strings.ToLower(u.Hostname())
	return domaine, u.Scheme + "://" + u.Host, mfa.PasskeyPossible(domaine)
}

func cleMemoire(prefixe, jeton string) string {
	h := sha256.Sum256([]byte(jeton))
	return prefixe + "|" + hex.EncodeToString(h[:])
}

func ecrireJSON(w http.ResponseWriter, statut int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statut)
	_ = json.NewEncoder(w).Encode(v)
}

func erreurJSON(w http.ResponseWriter, statut int, message string) {
	ecrireJSON(w, statut, map[string]string{"erreur": message})
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// secondFacteur : le compte demande-t-il un second facteur ?
func (a *App) secondFacteur(ctx context.Context, m donnees.Membre) (totp, passkeys bool, err error) {
	n, err := a.Q.CompterPasskeys(ctx, m.ID)
	return m.Totp != "", n > 0, err
}

// terminerConnexion ouvre la session (dernière étape de toute connexion).
func (a *App) terminerConnexion(w http.ResponseWriter, r *http.Request, m donnees.Membre, comment string) error {
	if err := a.OuvrirSession(r.Context(), w, m); err != nil {
		return err
	}
	// L'appareil d'un parent devient un appareil de confiance : les enfants
	// pourront s'y connecter avec leur code PIN.
	if m.Role == RoleParent {
		if err := a.MarquerAppareil(r.Context(), w, r, m.FamilleID); err != nil {
			a.Journal.Error("appareil de confiance", "erreur", err)
		}
	}
	a.Limiteur.Effacer("compte|" + strings.ToLower(m.Identifiant.String))
	a.Journal.Info("connexion", "membre", m.ID, "par", comment, "ip", a.IPClient(r).String())
	return nil
}

// debuterSecondFacteur : mot de passe bon, on demande le second facteur.
func (a *App) debuterSecondFacteur(w http.ResponseWriter, r *http.Request, m donnees.Membre, suite string) error {
	jeton, _ := jetonAleatoire()
	if err := a.Memoire.Mettre(cleMemoire("attente", jeton), &attenteConnexion{Membre: m.ID, Suite: suite}, dureeAttente); err != nil {
		return err
	}
	a.poserCookie(w, "fc_attente", jeton, dureeAttente)
	http.Redirect(w, r, "/connexion/verification", http.StatusSeeOther)
	return nil
}

// attenteDe : la connexion en attente de cette requête (et le membre).
func (a *App) attenteDe(r *http.Request) (*attenteConnexion, donnees.Membre, bool) {
	ck, err := r.Cookie(a.nomCookie("fc_attente"))
	if err != nil {
		return nil, donnees.Membre{}, false
	}
	v, ok := a.Memoire.Lire(cleMemoire("attente", ck.Value))
	if !ok {
		return nil, donnees.Membre{}, false
	}
	at := v.(*attenteConnexion)
	m, err := a.Q.MembreParIDSeul(r.Context(), at.Membre)
	if err != nil || !m.Mdp.Valid {
		return nil, donnees.Membre{}, false
	}
	return at, m, true
}

func (a *App) finirAttente(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(a.nomCookie("fc_attente")); err == nil {
		a.Memoire.Effacer(cleMemoire("attente", ck.Value))
	}
	a.effacerCookie(w, "fc_attente")
}

func (a *App) routesMFA() {
	a.Mux.HandleFunc("GET /connexion/verification", a.pageVerification)
	a.Mux.HandleFunc("POST /connexion/verification", a.verifierCode)
	a.Mux.HandleFunc("GET /connexion/verification/defi", a.defiVerification)
	a.Mux.HandleFunc("POST /connexion/verification/passkey", a.verifierPasskeyAttente)
	a.Mux.HandleFunc("GET /connexion/passkey/defi", a.defiSansMotDePasse)
	a.Mux.HandleFunc("POST /connexion/passkey", a.connexionPasskey)

	a.Route("GET /moi/securite", "", Membre, a.pageSecurite)
	a.Route("POST /moi/securite/totp/activer", "", Membre, a.activerTOTP)
	a.Route("POST /moi/securite/totp/desactiver", "", Membre, a.desactiverTOTP)
	a.Route("POST /moi/securite/codes", "", Membre, a.nouveauxCodes)
	// Ouvertes directement (rechargement, favori) : retour à la page Sécurité.
	for _, chemin := range []string{"/moi/securite/totp/activer", "/moi/securite/codes", "/moi/securite/totp/desactiver"} {
		a.Mux.HandleFunc("GET "+chemin, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/moi/securite", http.StatusSeeOther)
		})
	}
	a.Route("GET /moi/securite/passkeys/defi", "", Membre, a.defiInscription)
	a.Route("POST /moi/securite/passkeys", "", Membre, a.ajouterPasskey)
	a.Route("POST /moi/securite/passkeys/{id}/supprimer", "", Membre, a.supprimerPasskey)
	a.Route("POST /famille/membres/{id}/securite", "", Parent, a.reinitialiserSecurite)
}

func (a *App) afficherVerification(w http.ResponseWriter, r *http.Request, statut int, m donnees.Membre, erreur string) {
	totp, passkeys, _ := a.secondFacteur(r.Context(), m)
	a.afficherAnonyme(w, r, statut, "comptes/verification", "Vérification",
		formConnexion{Prenom: m.Nom, Erreur: erreur, TOTP: totp, Passkeys: passkeys})
}

func (a *App) pageVerification(w http.ResponseWriter, r *http.Request) {
	_, m, ok := a.attenteDe(r)
	if !ok {
		http.Redirect(w, r, "/connexion", http.StatusSeeOther)
		return
	}
	a.afficherVerification(w, r, http.StatusOK, m, "")
}

// verifierCode : code de l'application ou code de secours.
func (a *App) verifierCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	at, m, ok := a.attenteDe(r)
	if !ok {
		http.Redirect(w, r, "/connexion", http.StatusSeeOther)
		return
	}
	ip := a.IPClient(r)
	fin, attente, ok := a.Limiteur.Essayer(time.Now(), Limite{CleIP(ip), seuilIP}, Limite{"mfa|" + strconv.FormatInt(m.ID, 10), seuilMFA})
	if !ok {
		a.Journal.Warn("second facteur refusé : trop d'essais", "membre", m.ID, "ip", ip.String())
		a.afficherVerification(w, r, http.StatusTooManyRequests, m, "Trop d'essais. Réessayez dans "+attenteLisible(attente)+".")
		return
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	bon := false
	switch {
	case mfa.EstCodeSecours(code):
		n, err := a.Q.UtiliserCodeSecours(ctx, donnees.UtiliserCodeSecoursParams{UtiliseLe: time.Now().Unix(), MembreID: m.ID, Empreinte: mfa.EmpreinteSecours(code)})
		if err != nil {
			fin(false)
			a.ErreurInterne(w, r, err)
			return
		}
		if bon = n == 1; bon {
			a.Journal.Warn("code de secours utilisé", "membre", m.ID, "ip", ip.String())
		}
	case m.Totp != "":
		if pas, valide := mfa.VerifierTOTP(m.Totp, code, time.Now(), m.TotpDernier); valide {
			n, err := a.Q.NoterPasTOTP(ctx, donnees.NoterPasTOTPParams{Pas: pas, ID: m.ID})
			if err != nil {
				fin(false)
				a.ErreurInterne(w, r, err)
				return
			}
			bon = n == 1 // sinon : code déjà utilisé par une requête simultanée
		}
	}
	if !bon {
		fin(true)
		a.Journal.Warn("mauvais code de double authentification", "membre", m.ID, "ip", ip.String())
		a.afficherVerification(w, r, http.StatusUnauthorized, m, "Code incorrect.")
		return
	}
	fin(false)
	a.Limiteur.Effacer("mfa|" + strconv.FormatInt(m.ID, 10))
	a.finirAttente(w, r)
	if err := a.terminerConnexion(w, r, m, "mot de passe + code"); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	http.Redirect(w, r, at.Suite, http.StatusSeeOther)
}

// optionsConnexion : ce qu'attend navigator.credentials.get (en base64url,
// converti par app.js).
func optionsConnexion(defi, domaine, verification string, cles []donnees.Passkey) map[string]any {
	autorisees := []map[string]string{}
	for _, p := range cles {
		autorisees = append(autorisees, map[string]string{"type": "public-key", "id": b64(p.Identifiant)})
	}
	return map[string]any{"challenge": defi, "rpId": domaine, "timeout": 120000,
		"userVerification": verification, "allowCredentials": autorisees}
}

func (a *App) defiVerification(w http.ResponseWriter, r *http.Request) {
	at, m, ok := a.attenteDe(r)
	domaine, _, possible := a.domaineWebAuthn()
	if !ok || !possible {
		erreurJSON(w, http.StatusBadRequest, "Connexion expirée : recommencez.")
		return
	}
	cles, err := a.Q.PasskeysMembre(r.Context(), m.ID)
	if err != nil || len(cles) == 0 {
		erreurJSON(w, http.StatusBadRequest, "Aucune passkey sur ce compte.")
		return
	}
	defi, _ := mfa.NouveauDefiPasskey()
	at.Defi = defi
	ecrireJSON(w, http.StatusOK, optionsConnexion(defi, domaine, "preferred", cles))
}

// reponseConnexion : la réponse de navigator.credentials.get, en base64url.
type reponseConnexion struct {
	ID                string `json:"id"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
}

// verifierAssertion contrôle une réponse de passkey et renvoie la passkey.
func (a *App) verifierAssertion(ctx context.Context, rep reponseConnexion, defi string, exigerVerification bool) (donnees.Passkey, error) {
	domaine, origine, _ := a.domaineWebAuthn()
	id, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(rep.ID, "="))
	if err != nil || len(id) == 0 || len(id) > 1023 {
		return donnees.Passkey{}, mfa.ErrPasskey
	}
	p, err := a.Q.PasskeyParIdentifiant(ctx, id)
	if err != nil {
		return p, fmt.Errorf("%w : passkey inconnue", mfa.ErrPasskey)
	}
	cle := &mfa.Passkey{Identifiant: p.Identifiant, ClePublique: p.ClePublique, Algorithme: int(p.Algorithme), Compteur: uint32(p.Compteur), Domaine: p.Domaine} // #nosec G115 -- compteur sur 32 bits
	compteur, err := mfa.VerifierConnexion(cle, rep.ClientDataJSON, rep.AuthenticatorData, rep.Signature, defi, domaine, origine, exigerVerification)
	if err != nil {
		return p, err
	}
	return p, a.Q.NoterPasskey(ctx, donnees.NoterPasskeyParams{Compteur: int64(compteur), UtiliseLe: time.Now().Unix(), ID: p.ID})
}

func lireReponse(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		erreurJSON(w, http.StatusBadRequest, "Réponse illisible.")
		return false
	}
	return true
}

// verifierPasskeyAttente : passkey en second facteur, après le mot de passe.
func (a *App) verifierPasskeyAttente(w http.ResponseWriter, r *http.Request) {
	at, m, ok := a.attenteDe(r)
	if !ok || at.Defi == "" {
		erreurJSON(w, http.StatusBadRequest, "Connexion expirée : recommencez.")
		return
	}
	var rep reponseConnexion
	if !lireReponse(w, r, &rep) {
		return
	}
	defi := at.Defi
	at.Defi = "" // usage unique
	ip := a.IPClient(r)
	p, err := a.verifierAssertion(r.Context(), rep, defi, false)
	if err != nil || p.MembreID != m.ID {
		a.Journal.Warn("passkey refusée", "membre", m.ID, "ip", ip.String(), "erreur", err)
		erreurJSON(w, http.StatusUnauthorized, "Passkey refusée.")
		return
	}
	a.finirAttente(w, r)
	if err := a.terminerConnexion(w, r, m, "mot de passe + passkey"); err != nil {
		erreurJSON(w, http.StatusInternalServerError, "Erreur interne.")
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]string{"suite": at.Suite})
}

// defiSansMotDePasse : connexion par passkey seule (bouton de la page de
// connexion). Le défi est rangé sous un cookie à usage unique.
func (a *App) defiSansMotDePasse(w http.ResponseWriter, r *http.Request) {
	domaine, _, possible := a.domaineWebAuthn()
	if !possible {
		erreurJSON(w, http.StatusBadRequest, "Passkeys indisponibles sur une adresse IP : utilisez un nom de domaine.")
		return
	}
	defi, _ := mfa.NouveauDefiPasskey()
	jeton, _ := jetonAleatoire()
	if err := a.Memoire.Mettre(cleMemoire("defi", jeton), defi, dureeAttente); err != nil {
		erreurJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	a.poserCookie(w, "fc_defi", jeton, dureeAttente)
	ecrireJSON(w, http.StatusOK, optionsConnexion(defi, domaine, "required", nil))
}

func (a *App) connexionPasskey(w http.ResponseWriter, r *http.Request) {
	ip := a.IPClient(r)
	ck, err := r.Cookie(a.nomCookie("fc_defi"))
	if err != nil {
		erreurJSON(w, http.StatusBadRequest, "Connexion expirée : recommencez.")
		return
	}
	v, ok := a.Memoire.Prendre(cleMemoire("defi", ck.Value))
	a.effacerCookie(w, "fc_defi")
	if !ok {
		erreurJSON(w, http.StatusBadRequest, "Connexion expirée : recommencez.")
		return
	}
	fin, attente, ok := a.Limiteur.Essayer(time.Now(), Limite{CleIP(ip), seuilIP})
	if !ok {
		erreurJSON(w, http.StatusTooManyRequests, "Trop d'essais. Réessayez dans "+attenteLisible(attente)+".")
		return
	}
	var rep reponseConnexion
	if !lireReponse(w, r, &rep) {
		fin(false)
		return
	}
	// Sans mot de passe : l'appareil DOIT avoir vérifié la personne.
	p, err := a.verifierAssertion(r.Context(), rep, v.(string), true)
	var m donnees.Membre
	if err == nil {
		m, err = a.Q.MembreParIDSeul(r.Context(), p.MembreID)
	}
	if err != nil {
		fin(true)
		a.Journal.Warn("connexion par passkey refusée", "ip", ip.String(), "erreur", err)
		erreurJSON(w, http.StatusUnauthorized, "Passkey refusée.")
		return
	}
	fin(false)
	if err := a.terminerConnexion(w, r, m, "passkey"); err != nil {
		erreurJSON(w, http.StatusInternalServerError, "Erreur interne.")
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]string{"suite": "/"})
}

// --- Page « Sécurité » (Moi) --------------------------------------------------

type ligneCle struct {
	donnees.Passkey
	Cree, Utilise string
}

type donneesSecurite struct {
	TOTP            bool
	Restants        int64
	Secret          string       // secret proposé (activation en cours)
	QR              template.URL // image du QR code
	Passkeys        []ligneCle
	PasskeyPossible bool
	Codes           []string // codes de secours, montrés une seule fois
}

func (a *App) afficherSecurite(w http.ResponseWriter, r *http.Request, c *Contexte, codes []string) {
	ctx := r.Context()
	// Page affichée en réponse à un formulaire (activation, nouveaux codes) :
	// l'adresse affichée redevient /moi/securite, pour qu'un rechargement ne
	// redemande pas une adresse qui n'accepte que les formulaires.
	w.Header().Set("HX-Push-Url", "/moi/securite")
	if !c.Membre.Mdp.Valid {
		a.PageErreur(w, r, c, http.StatusBadRequest, "Pas besoin", "Ce compte se connecte avec un code PIN, sur un appareil de confiance.")
		return
	}
	m, err := a.Q.MembreParIDSeul(ctx, c.Membre.ID) // état à jour (après activation)
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	_, _, possible := a.domaineWebAuthn()
	d := donneesSecurite{TOTP: m.Totp != "", Codes: codes, PasskeyPossible: possible}
	if d.TOTP {
		d.Restants, _ = a.Q.CodesSecoursRestants(ctx, m.ID)
	} else {
		// Secret proposé, gardé 10 minutes le temps de scanner le QR code.
		cle := cleMemoire("totp", string(c.jeton))
		if v, ok := a.Memoire.Lire(cle); ok {
			d.Secret = v.(string)
		} else {
			d.Secret, _ = mfa.NouveauSecretTOTP()
			if err := a.Memoire.Mettre(cle, d.Secret, 10*time.Minute); err != nil {
				a.Echec(w, r, http.StatusServiceUnavailable, err.Error())
				return
			}
		}
		d.QR, _ = mfa.ImageQR(mfa.URITOTP(m.Identifiant.String, d.Secret))
	}
	cles, err := a.Q.PasskeysMembre(ctx, m.ID)
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	for _, p := range cles {
		l := ligneCle{Passkey: p, Cree: DateLongue(c.Heure(p.CreeLe))}
		if p.UtiliseLe > 0 {
			l.Utilise = DateLongue(c.Heure(p.UtiliseLe))
		}
		d.Passkeys = append(d.Passkeys, l)
	}
	a.Afficher(w, r, c, "securite", "Sécurité", "", d)
}

func (a *App) pageSecurite(w http.ResponseWriter, r *http.Request, c *Contexte) {
	a.afficherSecurite(w, r, c, nil)
}

// creerCodes remplace les codes de secours et renvoie les nouveaux.
func (a *App) creerCodes(ctx context.Context, membre int64) ([]string, error) {
	codes := mfa.NouveauxCodesSecours()
	err := a.Transaction(ctx, func(q *donnees.Queries) error {
		if err := q.EffacerCodesSecours(ctx, membre); err != nil {
			return err
		}
		for _, code := range codes {
			if err := q.AjouterCodeSecours(ctx, donnees.AjouterCodeSecoursParams{MembreID: membre, Empreinte: mfa.EmpreinteSecours(code), CreeLe: time.Now().Unix()}); err != nil {
				return err
			}
		}
		return nil
	})
	return codes, err
}

func (a *App) activerTOTP(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	if !c.Membre.Mdp.Valid || c.Membre.Totp != "" {
		a.Echec(w, r, http.StatusBadRequest, "Double authentification déjà active.")
		return
	}
	v, ok := a.Memoire.Lire(cleMemoire("totp", string(c.jeton)))
	if !ok {
		a.Echec(w, r, http.StatusBadRequest, "Trop tard : rechargez la page et scannez le nouveau QR code.")
		return
	}
	secret := v.(string)
	pas, valide := mfa.VerifierTOTP(secret, r.PostFormValue("code"), time.Now(), 0)
	if !valide {
		a.Echec(w, r, http.StatusBadRequest, "Code incorrect : vérifiez l'heure du téléphone et réessayez.")
		return
	}
	if err := a.Q.ActiverTOTP(ctx, donnees.ActiverTOTPParams{Totp: secret, TotpDernier: pas, ID: c.Membre.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Memoire.Effacer(cleMemoire("totp", string(c.jeton)))
	codes, err := a.creerCodes(ctx, c.Membre.ID)
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Info("double authentification activée", "membre", c.Membre.ID)
	a.afficherSecurite(w, r, c, codes)
}

// motDePasseConfirme : le mot de passe actuel, redemandé avant une action sensible.
func (a *App) motDePasseConfirme(w http.ResponseWriter, r *http.Request, c *Contexte) bool {
	bon, occupe := ComparerOccupe(c.Membre.Mdp.String, r.PostFormValue("mdp"))
	switch {
	case occupe:
		a.Echec(w, r, http.StatusServiceUnavailable, "Serveur occupé : réessayez dans un instant.")
	case !bon:
		a.Journal.Warn("confirmation par mot de passe ratée", "membre", c.Membre.ID, "ip", a.IPClient(r).String())
		a.Echec(w, r, http.StatusBadRequest, "Mot de passe incorrect.")
	}
	return bon && !occupe
}

func (a *App) desactiverTOTP(w http.ResponseWriter, r *http.Request, c *Contexte) {
	if !a.motDePasseConfirme(w, r, c) {
		return
	}
	ctx := r.Context()
	err := a.Transaction(ctx, func(q *donnees.Queries) error {
		if err := q.DesactiverTOTP(ctx, c.Membre.ID); err != nil {
			return err
		}
		return q.EffacerCodesSecours(ctx, c.Membre.ID)
	})
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Warn("double authentification désactivée", "membre", c.Membre.ID, "ip", a.IPClient(r).String())
	a.Flash(w, "Double authentification désactivée")
	a.Rediriger(w, r, "/moi/securite")
}

func (a *App) nouveauxCodes(w http.ResponseWriter, r *http.Request, c *Contexte) {
	if c.Membre.Totp == "" {
		a.Echec(w, r, http.StatusBadRequest, "Activez d'abord la double authentification.")
		return
	}
	if !a.motDePasseConfirme(w, r, c) {
		return
	}
	codes, err := a.creerCodes(r.Context(), c.Membre.ID)
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Info("nouveaux codes de secours", "membre", c.Membre.ID)
	a.afficherSecurite(w, r, c, codes)
}

// defiInscription : options de navigator.credentials.create.
func (a *App) defiInscription(w http.ResponseWriter, r *http.Request, c *Contexte) {
	domaine, _, possible := a.domaineWebAuthn()
	if !possible || !c.Membre.Mdp.Valid {
		erreurJSON(w, http.StatusBadRequest, "Passkeys indisponibles ici (adresse IP ou compte à code PIN).")
		return
	}
	cles, err := a.Q.PasskeysMembre(r.Context(), c.Membre.ID)
	if err != nil {
		erreurJSON(w, http.StatusInternalServerError, "Erreur interne.")
		return
	}
	if len(cles) >= mfa.PasskeysMax {
		erreurJSON(w, http.StatusBadRequest, fmt.Sprintf("%d passkeys au plus : supprimez-en une.", mfa.PasskeysMax))
		return
	}
	defi, _ := mfa.NouveauDefiPasskey()
	if err := a.Memoire.Mettre(cleMemoire("inscription", string(c.jeton)), defi, dureeAttente); err != nil {
		erreurJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	exclues := []map[string]string{}
	for _, p := range cles {
		exclues = append(exclues, map[string]string{"type": "public-key", "id": b64(p.Identifiant)})
	}
	algos := []map[string]any{}
	for _, alg := range mfa.AlgorithmesPasskey {
		algos = append(algos, map[string]any{"type": "public-key", "alg": alg})
	}
	// Identifiant WebAuthn du compte : opaque (pas le nom).
	h := sha256.Sum256([]byte("familycocon-membre:" + strconv.FormatInt(c.Membre.ID, 10)))
	ecrireJSON(w, http.StatusOK, map[string]any{
		"challenge": defi,
		"rp":        map[string]string{"id": domaine, "name": "FamilyCocon"},
		"user": map[string]string{"id": b64(h[:16]), "name": c.Membre.Identifiant.String,
			"displayName": c.Membre.Nom + " (" + c.Famille.Nom + ")"},
		"pubKeyCredParams":       algos,
		"timeout":                120000,
		"attestation":            "none",
		"excludeCredentials":     exclues,
		"authenticatorSelection": map[string]any{"residentKey": "preferred", "userVerification": "preferred"},
	})
}

// ajouterPasskey enregistre la passkey créée par le navigateur.
func (a *App) ajouterPasskey(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	var rep struct {
		ClientDataJSON    string `json:"clientDataJSON"`
		AttestationObject string `json:"attestationObject"`
		Nom               string `json:"nom"`
	}
	if !lireReponse(w, r, &rep) {
		return
	}
	v, ok := a.Memoire.Prendre(cleMemoire("inscription", string(c.jeton)))
	if !ok {
		erreurJSON(w, http.StatusBadRequest, "Trop tard : recommencez.")
		return
	}
	domaine, origine, _ := a.domaineWebAuthn()
	p, err := mfa.VerifierCreation(rep.ClientDataJSON, rep.AttestationObject, v.(string), domaine, origine)
	if err != nil {
		a.Journal.Warn("passkey refusée à l'enregistrement", "membre", c.Membre.ID, "erreur", err)
		erreurJSON(w, http.StatusBadRequest, "Passkey refusée.")
		return
	}
	nom := mfa.NomPasskey(rep.Nom)
	err = a.Transaction(ctx, func(q *donnees.Queries) error {
		if n, err := q.CompterPasskeys(ctx, c.Membre.ID); err != nil || n >= mfa.PasskeysMax {
			return errors.Join(err, errTropDeCles)
		}
		return q.AjouterPasskey(ctx, donnees.AjouterPasskeyParams{
			MembreID: c.Membre.ID, Identifiant: p.Identifiant, ClePublique: p.ClePublique, Algorithme: int64(p.Algorithme),
			Compteur: int64(p.Compteur), Domaine: p.Domaine, Nom: nom, CreeLe: time.Now().Unix(),
		})
	})
	switch {
	case errors.Is(err, errTropDeCles):
		erreurJSON(w, http.StatusBadRequest, "Trop de passkeys.")
		return
	case estDoublon(err):
		erreurJSON(w, http.StatusConflict, "Cette passkey est déjà enregistrée.")
		return
	case err != nil:
		erreurJSON(w, http.StatusInternalServerError, "Erreur interne.")
		return
	}
	a.Journal.Info("passkey ajoutée", "membre", c.Membre.ID, "nom", nom)
	a.Flash(w, "Passkey ajoutée 🔑")
	w.WriteHeader(http.StatusNoContent)
}

var errTropDeCles = errors.New("trop de passkeys")

func (a *App) supprimerPasskey(w http.ResponseWriter, r *http.Request, c *Contexte) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	n, err := a.Q.SupprimerPasskey(r.Context(), donnees.SupprimerPasskeyParams{ID: id, MembreID: c.Membre.ID})
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	if n == 1 {
		a.Journal.Info("passkey supprimée", "membre", c.Membre.ID)
	}
	a.Flash(w, "Passkey supprimée")
	a.Rediriger(w, r, "/moi/securite")
}

// reinitialiserSecurite : un parent retire la double authentification et
// les passkeys d'un membre (téléphone perdu), et le déconnecte partout.
func (a *App) reinitialiserSecurite(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	m, err := a.membreCible(r, c)
	if err != nil {
		a.Echec(w, r, http.StatusNotFound, "Membre introuvable.")
		return
	}
	err = a.Transaction(ctx, func(q *donnees.Queries) error {
		for _, f := range []func(context.Context, int64) error{q.DesactiverTOTP, q.EffacerCodesSecours, q.SupprimerPasskeysMembre, q.SupprimerSessionsMembre} {
			if err := f(ctx, m.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Warn("sécurité réinitialisée par un parent", "membre", m.ID, "par", c.Membre.ID)
	a.Flash(w, "Double authentification et passkeys de "+m.Nom+" retirées")
	a.Rediriger(w, r, "/famille")
}
