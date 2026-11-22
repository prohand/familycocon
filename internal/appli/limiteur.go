// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"net/netip"
	"sync"
	"time"
)

// Limite des tentatives de connexion (mot de passe, code PIN), en mémoire
// (idée reprise de mirador-stack, internal/server/auth/verrou.go, en plus
// simple) :
//   - chaque clé (« ip|… », « compte|… », « pin|… ») a son seuil d'échecs
//     dans une fenêtre de 15 minutes ;
//   - le seuil atteint, la clé est verrouillée : 15 min, puis le double à
//     chaque nouveau verrou (24 h au plus) ; la série repart de zéro après
//     24 h sans verrou, ou après une connexion réussie pour un compte ;
//   - un essai est RÉSERVÉ avant la vérification : des requêtes envoyées
//     toutes en même temps ne permettent pas plus d'essais que le seuil ;
//   - la table est bornée : au-delà de 10 000 clés, les compteurs périmés
//     puis les plus anciens non verrouillés sont oubliés.
// Rien n'est écrit sur disque ; chaque échec est écrit dans le journal
// (lisible par fail2ban ou CrowdSec).

const (
	fenetreEchecs = 15 * time.Minute
	verrouBase    = 15 * time.Minute
	verrouMax     = 24 * time.Hour
	oubliSerie    = 24 * time.Hour
	clesMax       = 10000
)

// Limite : une clé et son seuil.
type Limite struct {
	Cle   string
	Seuil int
}

type compteur struct {
	echecs   int
	premier  time.Time
	jusqua   time.Time // verrouillé jusqu'à
	serie    int
	finSerie time.Time
}

// Limiteur : compteurs d'échecs.
type Limiteur struct {
	mu      sync.Mutex
	cles    map[string]*compteur
	enCours map[string]int
}

// NouveauLimiteur : table vide.
func NouveauLimiteur() *Limiteur {
	return &Limiteur{cles: map[string]*compteur{}, enCours: map[string]int{}}
}

// recents : échecs comptés dans la fenêtre (0 si la fenêtre est passée).
func (c *compteur) recents(t time.Time) int {
	if c == nil || t.Sub(c.premier) > fenetreEchecs {
		return 0
	}
	return c.echecs
}

// Essayer réserve un essai pour toutes les limites. Refusé si l'une est
// verrouillée, ou si ses échecs récents plus les essais en cours atteignent
// son seuil ; attente dit alors combien de temps patienter (au moins).
// Accepté : appeler fin(echec) une fois la vérification faite ; echec = vrai
// compte un échec pour chaque limite. Après une connexion réussie, Effacer
// remet le compteur du compte à zéro.
func (l *Limiteur) Essayer(t time.Time, limites ...Limite) (fin func(echec bool), attente time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, li := range limites {
		c := l.cles[li.Cle]
		if c != nil && t.Before(c.jusqua) {
			return nil, c.jusqua.Sub(t), false
		}
		if c.recents(t)+l.enCours[li.Cle] >= li.Seuil {
			return nil, time.Minute, false
		}
	}
	for _, li := range limites {
		l.enCours[li.Cle]++
	}
	fini := false
	return func(echec bool) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if fini {
			return
		}
		fini = true
		for _, li := range limites {
			if l.enCours[li.Cle]--; l.enCours[li.Cle] <= 0 {
				delete(l.enCours, li.Cle)
			}
			if echec {
				l.echec(li, t)
			}
		}
	}, 0, true
}

// Effacer oublie le compteur d'une clé (connexion réussie : « compte|… »).
// Le compteur d'une adresse n'est jamais effacé ainsi : un attaquant
// connecté ne regagne pas d'essais.
func (l *Limiteur) Effacer(cle string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.cles, cle)
}

func (l *Limiteur) echec(li Limite, t time.Time) {
	c := l.cles[li.Cle]
	if c == nil {
		if len(l.cles) >= clesMax {
			l.faireDeLaPlace(t)
		}
		c = &compteur{}
		l.cles[li.Cle] = c
	}
	if t.Sub(c.premier) > fenetreEchecs {
		c.echecs, c.premier = 0, t
	}
	c.echecs++
	if c.echecs < li.Seuil {
		return
	}
	if t.After(c.finSerie) {
		c.serie = 0
	}
	duree := verrouBase << c.serie
	if duree > verrouMax || duree <= 0 {
		duree = verrouMax
	}
	c.serie++
	c.jusqua = t.Add(duree)
	c.finSerie = c.jusqua.Add(oubliSerie)
	c.echecs, c.premier = 0, time.Time{}
}

// faireDeLaPlace : oublie les compteurs périmés, puis, s'il le faut, les
// plus anciens qui ne sont pas verrouillés.
func (l *Limiteur) faireDeLaPlace(t time.Time) {
	for k, c := range l.cles {
		if t.After(c.jusqua) && t.After(c.finSerie) && t.Sub(c.premier) > fenetreEchecs {
			delete(l.cles, k)
		}
	}
	for k, c := range l.cles {
		if len(l.cles) < clesMax*9/10 {
			return
		}
		if t.After(c.jusqua) {
			delete(l.cles, k)
		}
	}
}

// LeverVerrous efface tout (tests, ou redémarrage).
func (l *Limiteur) LeverVerrous() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cles = map[string]*compteur{}
}

// CleIP : l'adresse (IPv4) ou son /64 (IPv6) : un attaquant qui dispose
// d'un /64 ne peut pas changer d'adresse à chaque essai.
func CleIP(ip netip.Addr) string {
	if ip.Is6() && !ip.Is4In6() {
		if p, err := ip.Prefix(64); err == nil {
			return "ip|" + p.String()
		}
	}
	return "ip|" + ip.String()
}
