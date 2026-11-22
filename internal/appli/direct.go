// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
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

// Direct : les abonnés, rangés par famille.
type Direct struct {
	mu      sync.Mutex
	abonnes map[int64][]chan []byte
	ferme   bool
}

func nouveauDirect() *Direct { return &Direct{abonnes: map[int64][]chan []byte{}} }

func (d *Direct) abonner(famille int64) chan []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	ch := make(chan []byte, 16)
	if d.ferme {
		close(ch)
		return ch
	}
	l := d.abonnes[famille]
	if len(l) >= abonnesMax {
		close(l[0])
		l = l[1:]
	}
	d.abonnes[famille] = append(l, ch)
	return ch
}

func (d *Direct) desabonner(famille int64, ch chan []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	l := d.abonnes[famille]
	for i, x := range l {
		if x == ch {
			d.abonnes[famille] = append(l[:i:i], l[i+1:]...)
			close(ch)
			break
		}
	}
	if len(d.abonnes[famille]) == 0 {
		delete(d.abonnes, famille)
	}
}

// Publier envoie un message à toute la famille. Un navigateur trop lent
// (canal plein) perd le message : il sera à jour au prochain.
func (d *Direct) Publier(famille int64, m Message) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, ch := range d.abonnes[famille] {
		select {
		case ch <- b:
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
		for _, ch := range l {
			close(ch)
		}
		delete(d.abonnes, f)
	}
}

// Publier : raccourci pour un module (notification signée par le membre).
func (a *App) Publier(c *Contexte, module, texte string) {
	a.Direct.Publier(c.Famille.ID, Message{
		Module: module, Texte: texte, Auteur: c.Membre.ID,
		Couleur: c.Membre.Couleur, Initiale: Initiale(c.Membre.Nom),
	})
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

	ch := a.Direct.abonner(c.Famille.ID)
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
			if _, err := fmt.Fprint(w, ": pouls\n\n"); err != nil {
				return
			}
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
