// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// Gestionnaire : une page ou une action pour un membre connecté.
type Gestionnaire func(w http.ResponseWriter, r *http.Request, c *Contexte)

// Acces : qui peut utiliser une route.
type Acces int

const (
	// Membre : tout membre connecté (parent ou enfant).
	Membre Acces = iota
	// Parent : les parents seulement.
	Parent
)

// Route ajoute une route réservée aux membres connectés.
//   - motif : comme http.ServeMux (« GET /courses », « POST /courses/{id} ») ;
//   - module : clé du module ("" si la page n'appartient à aucun module) ;
//     un module désactivé répond 404 ;
//   - acces : Membre ou Parent.
//
// Pour toute requête qui modifie quelque chose (POST…), le jeton CSRF de
// la session est exigé, en plus du contrôle d'origine global.
func (a *App) Route(motif, module string, acces Acces, h Gestionnaire) {
	a.Mux.HandleFunc(motif, func(w http.ResponseWriter, r *http.Request) {
		c, err := a.lireSession(r)
		if err != nil {
			a.ErreurInterne(w, r, err)
			return
		}
		if c == nil {
			if r.Method == http.MethodGet {
				a.Rediriger(w, r, "/connexion?suite="+url.QueryEscape(r.URL.RequestURI()))
				return
			}
			a.Echec(w, r, http.StatusUnauthorized, "Session terminée : reconnectez-vous.")
			return
		}
		if module != "" && !c.Actif(module) {
			a.PageErreur(w, r, c, http.StatusNotFound, "Module désactivé", "Ce module n'est pas activé pour votre famille.")
			return
		}
		if acces == Parent && !c.EstParent() {
			a.PageErreur(w, r, c, http.StatusForbidden, "Réservé aux parents", "Demande à un parent 🙂")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !jetonCSRFValide(r, c.CSRF) {
			a.Journal.Warn("jeton CSRF absent ou faux", "chemin", r.URL.Path, "membre", c.Membre.ID)
			a.Echec(w, r, http.StatusForbidden, "Page trop ancienne : rechargez-la puis recommencez.")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), cleContexte{}, c)), c)
	})
}

// EstHtmx : requête envoyée par htmx (et pas un chargement de page complet).
func EstHtmx(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// Rediriger : redirection qui marche avec et sans htmx.
func (a *App) Rediriger(w http.ResponseWriter, r *http.Request, url string) {
	if EstHtmx(r) {
		// htmx charge la page suivante sans tout recharger, et met
		// l'adresse à jour dans le navigateur.
		w.Header().Set("HX-Location", url)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// RedirigerComplet : recharge toute la page (déconnexion : la mise en
// page change complètement).
func (a *App) RedirigerComplet(w http.ResponseWriter, r *http.Request, url string) {
	if EstHtmx(r) {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// Notifier : petite notification en bas de l'écran à la réponse de cette
// requête htmx (événement « fc-toast » lu par app.js).
func Notifier(w http.ResponseWriter, texte string, erreur bool) {
	b, _ := json.Marshal(map[string]any{"fc-toast": map[string]any{"texte": texte, "erreur": erreur}})
	w.Header().Set("HX-Trigger", string(b))
}

// Flash : notification affichée sur la PAGE SUIVANTE (après une redirection).
func (a *App) Flash(w http.ResponseWriter, texte string) {
	http.SetCookie(w, &http.Cookie{Name: "fc_flash", Value: url.QueryEscape(texte), Path: "/", MaxAge: 60,
		HttpOnly: true, Secure: a.Config.CookieSecurise(), SameSite: http.SameSiteLaxMode})
}

func (a *App) lireFlash(w http.ResponseWriter, r *http.Request) string {
	ck, err := r.Cookie("fc_flash")
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: "fc_flash", Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.Config.CookieSecurise(), SameSite: http.SameSiteLaxMode})
	t, _ := url.QueryUnescape(ck.Value)
	if len(t) > 200 {
		t = t[:200]
	}
	return t
}

// Echec : réponse d'erreur courte. Avec htmx : notification rouge, la page
// ne bouge pas ; sans htmx : page d'erreur.
func (a *App) Echec(w http.ResponseWriter, r *http.Request, statut int, message string) {
	if EstHtmx(r) {
		Notifier(w, message, true)
		w.Header().Set("HX-Reswap", "none")
		w.WriteHeader(statut)
		return
	}
	a.PageErreur(w, r, ContexteDe(r.Context()), statut, "Oups", message)
}

// ErreurInterne : écrit l'erreur dans le journal, répond 500 sans détail.
func (a *App) ErreurInterne(w http.ResponseWriter, r *http.Request, err error) {
	a.Journal.Error("erreur interne", "chemin", r.URL.Path, "erreur", err)
	a.Echec(w, r, http.StatusInternalServerError, "Erreur interne : réessayez dans un instant.")
}

// PageErreur : page d'erreur complète.
func (a *App) PageErreur(w http.ResponseWriter, r *http.Request, c *Contexte, statut int, titre, message string) {
	v := a.NouvelleVue(w, r, c, titre, "")
	v.Donnees = message
	if err := a.Gabarits.Executer(w, statut, "erreur", "base.html", v); err != nil {
		a.Journal.Error("page d'erreur", "erreur", err)
		http.Error(w, message, statut)
	}
}

// Afficher : page complète (mise en page commune + bloc « contenu »).
func (a *App) Afficher(w http.ResponseWriter, r *http.Request, c *Contexte, page, titre, module string, donnees any) {
	v := a.NouvelleVue(w, r, c, titre, module)
	v.Donnees = donnees
	if err := a.Gabarits.Executer(w, http.StatusOK, page, "base.html", v); err != nil {
		a.ErreurInterne(w, r, err)
	}
}

// Fragment : un seul bloc d'une page (réponse à htmx).
func (a *App) Fragment(w http.ResponseWriter, r *http.Request, c *Contexte, page, bloc string, donnees any) {
	v := a.NouvelleVue(nil, r, c, "", "")
	v.Donnees = donnees
	if err := a.Gabarits.Executer(w, http.StatusOK, page, bloc, v); err != nil {
		a.ErreurInterne(w, r, err)
	}
}

// Heure : l'heure de la requête dans le fuseau de la famille.
func (c *Contexte) Heure(unix int64) time.Time { return time.Unix(unix, 0).In(c.Lieu) }
