// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"
)

// Intergiciels (« middlewares ») appliqués à toutes les requêtes, de
// l'extérieur vers l'intérieur : secours (panique), journal, en-têtes de
// sécurité, taille maximale du corps, protection contre les requêtes
// venues d'un autre site (CSRF).

// csp : seuls les fichiers du site sont permis. Pas de script ni de style
// écrit dans la page (« inline ») : une injection ne pourrait rien exécuter.
const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"font-src 'self'; connect-src 'self'; manifest-src 'self'; worker-src 'self'; " +
	"frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'"

// TailleMaxCorps : 1 Mio, bien assez pour des formulaires.
const TailleMaxCorps = 1 << 20

// enregistreur garde le statut renvoyé, pour le journal.
type enregistreur struct {
	http.ResponseWriter
	statut int
}

func (e *enregistreur) WriteHeader(s int) {
	if e.statut == 0 {
		e.statut = s
	}
	e.ResponseWriter.WriteHeader(s)
}

func (e *enregistreur) Write(b []byte) (int, error) {
	if e.statut == 0 {
		e.statut = http.StatusOK
	}
	return e.ResponseWriter.Write(b)
}

// Unwrap : permet à http.ResponseController d'atteindre Flush (SSE).
func (e *enregistreur) Unwrap() http.ResponseWriter { return e.ResponseWriter }

// Envelopper applique les intergiciels communs autour de h.
func (a *App) Envelopper(h http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Journal.Warn("requête d'un autre site refusée (CSRF)", "chemin", r.URL.Path, "ip", a.IPClient(r).String(),
			"origine", r.Header.Get("Origin"))
		http.Error(w, "Requête refusée", http.StatusForbidden)
	}))
	interne := protection.Handler(h)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		debut := time.Now()
		e := &enregistreur{ResponseWriter: w}
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				a.Journal.Error("panique", "erreur", v, "chemin", r.URL.Path, "pile", string(debug.Stack()))
				if e.statut == 0 {
					http.Error(e, "Erreur interne", http.StatusInternalServerError)
				}
			}
			a.Journal.Log(r.Context(), niveauRequete(r.URL.Path, e.statut), "requête", "methode", r.Method, "chemin", r.URL.Path,
				"statut", e.statut, "duree_ms", time.Since(debut).Milliseconds(), "ip", a.IPClient(r).String())
		}()

		h := e.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if a.Config.CookieSecurise() {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		r.Body = http.MaxBytesReader(e, r.Body, TailleMaxCorps)
		interne.ServeHTTP(e, r)
	})
}

// IPClient : adresse du navigateur. Derrière un reverse proxy déclaré dans
// FC_PROXYS, on lit X-Forwarded-For de droite à gauche et on garde la
// première adresse qui n'est pas un proxy de confiance (un navigateur peut
// écrire ce qu'il veut à gauche de la liste).
func (a *App) IPClient(r *http.Request) netip.Addr {
	hote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		hote = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(hote)
	if err != nil {
		return netip.Addr{}
	}
	ip = ip.Unmap()
	if !a.deConfiance(ip) {
		return ip
	}
	liste := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(liste) - 1; i >= 0; i-- {
		suivante, err := netip.ParseAddr(strings.TrimSpace(liste[i]))
		if err != nil {
			break
		}
		ip = suivante.Unmap()
		if !a.deConfiance(ip) {
			break
		}
	}
	return ip
}

func (a *App) deConfiance(ip netip.Addr) bool {
	for _, p := range a.Config.Proxys {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// niveauRequete : le niveau de la ligne de journal d'une requête. Les
// requêtes qui réussissent sont en debug (sinon chaque page et chaque
// rafraîchissement remplirait le journal) ; les refus (401, 403, 404…) en
// info, utiles pour repérer des essais d'accès ; trop de tentatives (429)
// en warn ; les erreurs du serveur en error.
func niveauRequete(chemin string, statut int) slog.Level {
	switch {
	case statut >= 500:
		return slog.LevelError
	case statut == http.StatusTooManyRequests:
		return slog.LevelWarn
	case statut >= 400 && !strings.HasPrefix(chemin, "/statique/") && chemin != "/favicon.ico":
		return slog.LevelInfo
	}
	return slog.LevelDebug
}
