// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"familycocon/internal/donnees"
)

// Sessions : un jeton aléatoire (32 octets) dans un cookie HttpOnly,
// Secure (en https), SameSite=Lax. En base, seule son empreinte SHA-256 est
// gardée : une copie de la base ne permet pas de se connecter.

// Contexte : qui fait la requête.
type Contexte struct {
	Membre  donnees.Membre
	Famille donnees.Famille
	Lieu    *time.Location  // fuseau de la famille
	Actifs  map[string]bool // modules activés
	CSRF    string
	jeton   []byte // empreinte de la session
}

// EstParent : le membre est-il un parent ?
func (c *Contexte) EstParent() bool { return c.Membre.Role == RoleParent }

// Actif : le module est-il activé pour la famille ?
func (c *Contexte) Actif(module string) bool { return c.Actifs[module] }

// Maintenant : l'heure dans le fuseau de la famille.
func (c *Contexte) Maintenant() time.Time { return time.Now().In(c.Lieu) }

// Rôles.
const (
	RoleParent = "parent"
	RoleEnfant = "enfant"
)

type cleContexte struct{}

// ContexteDe : le contexte rangé dans la requête (nil si pas connecté).
func ContexteDe(ctx context.Context) *Contexte {
	c, _ := ctx.Value(cleContexte{}).(*Contexte)
	return c
}

// nomCookie : préfixe « __Host- » en https (le navigateur refuse alors
// tout cookie de même nom posé par un sous-domaine ou sans Secure).
func (a *App) nomCookie(nom string) string {
	if a.Config.CookieSecurise() {
		return "__Host-" + nom
	}
	return nom
}

func (a *App) poserCookie(w http.ResponseWriter, nom, valeur string, duree time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.nomCookie(nom),
		Value:    valeur,
		Path:     "/",
		MaxAge:   int(duree.Seconds()),
		HttpOnly: true,
		Secure:   a.Config.CookieSecurise(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) effacerCookie(w http.ResponseWriter, nom string) {
	http.SetCookie(w, &http.Cookie{Name: a.nomCookie(nom), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.Config.CookieSecurise(), SameSite: http.SameSiteLaxMode})
}

// jetonAleatoire : 32 octets aléatoires, en base64 (URL), et son empreinte.
func jetonAleatoire() (string, []byte) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // ne renvoie jamais d'erreur (Go ≥ 1.24)
	j := base64.RawURLEncoding.EncodeToString(b)
	return j, Empreinte256(j)
}

// Empreinte256 : empreinte SHA-256 d'un jeton.
func Empreinte256(jeton string) []byte {
	s := sha256.Sum256([]byte(jeton))
	return s[:]
}

// OuvrirSession crée une session pour le membre et pose le cookie.
func (a *App) OuvrirSession(ctx context.Context, w http.ResponseWriter, m donnees.Membre) error {
	jeton, empreinte := jetonAleatoire()
	csrf, _ := jetonAleatoire()
	t := time.Now()
	if err := a.Q.CreerSession(ctx, donnees.CreerSessionParams{
		Empreinte: empreinte, MembreID: m.ID, JetonCsrf: csrf,
		CreeLe: t.Unix(), VueLe: t.Unix(), ExpireLe: t.Add(a.Config.DureeSession).Unix(),
	}); err != nil {
		return err
	}
	a.poserCookie(w, "fc_session", jeton, a.Config.DureeSession)
	return nil
}

// FermerSession supprime la session de la requête et efface le cookie.
func (a *App) FermerSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(a.nomCookie("fc_session")); err == nil {
		_ = a.Q.SupprimerSession(r.Context(), Empreinte256(c.Value))
	}
	a.effacerCookie(w, "fc_session")
}

// lireSession : le contexte du membre connecté, ou nil.
func (a *App) lireSession(r *http.Request) (*Contexte, error) {
	ck, err := r.Cookie(a.nomCookie("fc_session"))
	if err != nil || len(ck.Value) != 43 {
		return nil, nil
	}
	empreinte := Empreinte256(ck.Value)
	t := time.Now()
	ligne, err := a.Q.SessionParEmpreinte(r.Context(), donnees.SessionParEmpreinteParams{Empreinte: empreinte, ExpireLe: t.Unix()})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Session glissante : prolongée au plus une fois par heure.
	if t.Unix()-ligne.Session.VueLe > 3600 {
		if err := a.Q.ProlongerSession(r.Context(), donnees.ProlongerSessionParams{
			VueLe: t.Unix(), ExpireLe: t.Add(a.Config.DureeSession).Unix(), Empreinte: empreinte,
		}); err != nil {
			return nil, err
		}
	}
	return a.NouveauContexte(ligne.Membre, ligne.Famille, ligne.Session.JetonCsrf, empreinte), nil
}

// NouveauContexte assemble un contexte (sert aussi aux tests).
func (a *App) NouveauContexte(m donnees.Membre, f donnees.Famille, csrf string, jeton []byte) *Contexte {
	lieu, err := time.LoadLocation(f.Fuseau)
	if err != nil {
		lieu = time.UTC
	}
	c := &Contexte{Membre: m, Famille: f, Lieu: lieu, CSRF: csrf, jeton: jeton, Actifs: map[string]bool{}}
	for _, mod := range strings.Split(f.Modules, ",") {
		c.Actifs[mod] = true
	}
	return c
}

// jetonCSRFValide : jeton envoyé par htmx (en-tête X-Jeton) ou par un
// formulaire classique (champ _jeton), comparé en temps constant.
func jetonCSRFValide(r *http.Request, attendu string) bool {
	recu := r.Header.Get("X-Jeton")
	if recu == "" {
		recu = r.PostFormValue("_jeton")
	}
	return recu != "" && subtle.ConstantTimeCompare([]byte(recu), []byte(attendu)) == 1
}

// Appareils de confiance : un cookie longue durée (400 jours) qui dit
// « cet appareil est celui de la famille n » ; on y propose la connexion
// des enfants par avatar + code PIN.

// MarquerAppareil pose le cookie d'appareil de confiance pour la famille.
func (a *App) MarquerAppareil(ctx context.Context, w http.ResponseWriter, r *http.Request, familleID int64) error {
	if f, ok := a.AppareilDe(r); ok && f == familleID {
		return nil
	}
	jeton, empreinte := jetonAleatoire()
	t := time.Now().Unix()
	if err := a.Q.CreerAppareil(ctx, donnees.CreerAppareilParams{Empreinte: empreinte, FamilleID: familleID, CreeLe: t, VueLe: t}); err != nil {
		return err
	}
	a.poserCookie(w, "fc_appareil", jeton, 400*24*time.Hour)
	return nil
}

// AppareilDe : la famille de cet appareil de confiance, s'il en est un.
func (a *App) AppareilDe(r *http.Request) (int64, bool) {
	ck, err := r.Cookie(a.nomCookie("fc_appareil"))
	if err != nil || len(ck.Value) != 43 {
		return 0, false
	}
	empreinte := Empreinte256(ck.Value)
	ap, err := a.Q.AppareilParEmpreinte(r.Context(), empreinte)
	if err != nil {
		return 0, false
	}
	if t := time.Now().Unix(); t-ap.VueLe > 24*3600 {
		_ = a.Q.VoirAppareil(r.Context(), donnees.VoirAppareilParams{VueLe: t, Empreinte: empreinte})
	}
	return ap.FamilleID, true
}
