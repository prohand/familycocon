// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"familycocon/internal/donnees"
)

// Temps réel : chaque page ouverte garde une connexion « Server-Sent Events »
// sur /direct. Quand un membre modifie quelque chose (ajoute du pain…), le
// serveur envoie un petit message à toute la famille :
//   - la page concernée se met à jour toute seule (htmx recharge le bloc) ;
//   - une notification s'affiche (« Marie a ajouté « Pain » 🥖 »).
// Un abonné ne coûte qu'un petit canal en mémoire.

// Message : ce qui est envoyé aux navigateurs (événement SSE « fc »).
type Message struct {
	Module   string `json:"module,omitempty"`   // page à mettre à jour (« courses »…)
	Texte    string `json:"texte,omitempty"`    // notification (vide : pas de notification)
	Auteur   int64  `json:"auteur,omitempty"`   // membre à l'origine (il ne voit pas sa propre notification)
	Couleur  int64  `json:"couleur,omitempty"`  // couleur de l'avatar de la notification
	Initiale string `json:"initiale,omitempty"` // lettre de l'avatar
}

// abonnesMax : connexions temps réel par famille (plusieurs onglets et
// appareils par personne). Au-delà, la plus ancienne est fermée.
const abonnesMax = 40

// abonne : une page ouverte.
type abonne struct {
	ch     chan []byte
	membre int64
}

// Direct : les abonnés, rangés par famille.
type Direct struct {
	mu      sync.Mutex
	abonnes map[int64][]abonne
	ferme   bool
}

func nouveauDirect() *Direct { return &Direct{abonnes: map[int64][]abonne{}} }

func (d *Direct) abonner(famille, membre int64) chan []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	ch := make(chan []byte, 16)
	if d.ferme {
		close(ch)
		return ch
	}
	l := d.abonnes[famille]
	if len(l) >= abonnesMax {
		close(l[0].ch)
		l = l[1:]
	}
	d.abonnes[famille] = append(l, abonne{ch: ch, membre: membre})
	return ch
}

// FermerMembre : ferme les pages ouvertes d'un membre (retiré, sécurité
// réinitialisée) ; elles ne reçoivent plus rien.
func (d *Direct) FermerMembre(famille, membre int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var garde []abonne
	for _, x := range d.abonnes[famille] {
		if x.membre == membre {
			close(x.ch)
			continue
		}
		garde = append(garde, x)
	}
	if len(garde) == 0 {
		delete(d.abonnes, famille)
	} else {
		d.abonnes[famille] = garde
	}
}

func (d *Direct) desabonner(famille int64, ch chan []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	l := d.abonnes[famille]
	for i, x := range l {
		if x.ch == ch {
			d.abonnes[famille] = append(l[:i:i], l[i+1:]...)
			close(ch)
			break
		}
	}
	if len(d.abonnes[famille]) == 0 {
		delete(d.abonnes, famille)
	}
}

// EnLigne : le membre a-t-il une page ouverte (il voit déjà les
// notifications à l'écran : pas besoin de notification push) ?
func (d *Direct) EnLigne(famille, membre int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, x := range d.abonnes[famille] {
		if x.membre == membre {
			return true
		}
	}
	return false
}

// Publier envoie un message à toute la famille. Un navigateur trop lent
// (canal plein) perd le message : il sera à jour au prochain.
func (d *Direct) Publier(famille int64, m Message) { d.PublierPour(famille, m, nil) }

// PublierPour : comme Publier, mais la notification (le texte) ne s'affiche
// que chez les membres de pour (vide : toute la famille) ; les autres pages
// se mettent quand même à jour.
func (d *Direct) PublierPour(famille int64, m Message, pour []int64) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	sans := b
	if len(pour) > 0 && m.Texte != "" {
		m.Texte = ""
		if sans, err = json.Marshal(m); err != nil {
			return
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, x := range d.abonnes[famille] {
		msg := b
		if len(pour) > 0 && !slices.Contains(pour, x.membre) {
			msg = sans
		}
		select {
		case x.ch <- msg:
		default:
		}
	}
}

// Nombre : abonnés connectés (tests, supervision).
func (d *Direct) Nombre() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, l := range d.abonnes {
		n += len(l)
	}
	return n
}

// fermer termine toutes les connexions (arrêt du serveur).
func (d *Direct) fermer() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ferme = true
	for f, l := range d.abonnes {
		for _, x := range l {
			close(x.ch)
		}
		delete(d.abonnes, f)
	}
}

// Publier : raccourci pour un module (notification signée par le membre).
// Avec un texte, les membres qui n'ont pas de page ouverte reçoivent aussi
// une notification push (s'ils l'ont demandée pour ce module).
func (a *App) Publier(c *Contexte, module, texte string) { a.PublierPour(c, module, texte, nil) }

// PublierPour : comme Publier, mais la notification ne va qu'aux membres
// concernés (pour ; vide : toute la famille) : les participants d'un
// événement, l'enfant d'une mission, les parents pour une validation…
// Toutes les pages ouvertes se mettent à jour.
func (a *App) PublierPour(c *Contexte, module, texte string, pour []int64) {
	a.Direct.PublierPour(c.Famille.ID, Message{
		Module: module, Texte: texte, Auteur: c.Membre.ID,
		Couleur: c.Membre.Couleur, Initiale: Visage(c.Membre),
	}, pour)
	if texte != "" && a.Push != nil {
		if m, ok := ModuleParCle(module); ok && !m.Notifie {
			// Oubli à corriger dans le Catalogue : personne ne pourrait régler
			// cette notification dans Moi › Notifications.
			a.Journal.Warn("notification d'un module sans « Notifie »", "module", module)
		}
		a.Push.annoncer(annonce{famille: c.Famille.ID, auteur: c.Membre.ID, module: module, texte: texte, pour: pour})
	}
}

// Parents : les identifiants des parents de la famille (destinataires
// d'une notification « à valider »).
func (a *App) Parents(ctx context.Context, c *Contexte) []int64 {
	membres, err := a.Membres(ctx, c)
	if err != nil {
		return nil
	}
	var out []int64
	for _, m := range membres {
		if m.Role == RoleParent {
			out = append(out, m.ID)
		}
	}
	return out
}

// pouls : un commentaire SSE régulier garde la connexion ouverte à travers
// les reverse proxys (qui coupent une connexion muette).
const pouls = 25 * time.Second

func (a *App) direct(w http.ResponseWriter, r *http.Request, c *Contexte) {
	rc := http.NewResponseController(w)
	// Pas de délai d'écriture pour cette réponse sans fin.
	_ = rc.SetWriteDeadline(time.Time{})
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // Nginx : ne pas mettre en tampon
	w.WriteHeader(http.StatusOK)
	// retry : le navigateur se reconnecte après 5 s si la connexion tombe.
	if _, err := fmt.Fprint(w, "retry: 5000\n: bonjour\n\n"); err != nil {
		return
	}
	_ = rc.Flush()

	ch := a.Direct.abonner(c.Famille.ID, c.Membre.ID)
	defer a.Direct.desabonner(c.Famille.ID, ch)
	minuteur := time.NewTicker(pouls)
	defer minuteur.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "event: fc\ndata: %s\n\n", b); err != nil {
				return
			}
		case <-minuteur.C:
			// La session existe-t-elle encore (déconnexion, membre retiré,
			// sécurité réinitialisée) ? Sinon, plus rien n'est envoyé.
			if _, err := a.Q.SessionParEmpreinte(r.Context(), donnees.SessionParEmpreinteParams{Empreinte: c.jeton, ExpireLe: time.Now().Unix()}); err != nil {
				return
			}
			if _, err := fmt.Fprint(w, ": pouls\n\n"); err != nil {
				return
			}
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
