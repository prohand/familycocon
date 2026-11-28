// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"sync"
	"time"
)

// MemoireCourte : petites données qui ne vivent que quelques minutes
// (connexion en attente du second facteur, défi d'une passkey, secret TOTP
// pas encore confirmé). En mémoire seulement : un redémarrage les oublie,
// il suffit alors de recommencer. Bornée pour ne jamais grossir.
//
// Chaque entrée appartient à un groupe (un membre, une adresse IP) qui ne
// garde que ses parGroupe entrées les plus récentes : un seul compte (ou une
// seule adresse) ne peut pas remplir la mémoire. Pleine malgré tout, la
// mémoire oublie l'entrée qui expire le plus tôt au lieu de tout refuser :
// une avalanche de requêtes ne bloque plus les connexions des autres.
type MemoireCourte struct {
	mu        sync.Mutex
	entrees   map[string]entreeCourte
	max       int
	parGroupe int
}

type entreeCourte struct {
	valeur any
	expire time.Time
	groupe string
}

// NouvelleMemoireCourte : au plus max entrées, et parGroupe par groupe.
func NouvelleMemoireCourte(max, parGroupe int) *MemoireCourte {
	return &MemoireCourte{entrees: map[string]entreeCourte{}, max: max, parGroupe: parGroupe}
}

// Mettre range une valeur du groupe pour la durée donnée.
func (m *MemoireCourte) Mettre(groupe, cle string, valeur any, duree time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := time.Now()
	// Les entrées expirées partent, et l'on cherche celle qui expire le plus
	// tôt (dans le groupe, et en tout).
	n := 0
	var vieilleGroupe, vieille string
	for k, e := range m.entrees {
		if t.After(e.expire) {
			delete(m.entrees, k)
			continue
		}
		if k == cle {
			continue // remplacée plus bas
		}
		if vieille == "" || e.expire.Before(m.entrees[vieille].expire) {
			vieille = k
		}
		if e.groupe == groupe {
			n++
			if vieilleGroupe == "" || e.expire.Before(m.entrees[vieilleGroupe].expire) {
				vieilleGroupe = k
			}
		}
	}
	if _, existe := m.entrees[cle]; !existe {
		switch {
		case n >= m.parGroupe:
			delete(m.entrees, vieilleGroupe)
		case len(m.entrees) >= m.max:
			delete(m.entrees, vieille)
		}
	}
	m.entrees[cle] = entreeCourte{valeur: valeur, expire: t.Add(duree), groupe: groupe}
}

// Lire renvoie la valeur si elle n'a pas expiré.
func (m *MemoireCourte) Lire(cle string) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entrees[cle]
	if !ok || time.Now().After(e.expire) {
		delete(m.entrees, cle)
		return nil, false
	}
	return e.valeur, true
}

// Prendre lit et efface (usage unique : un défi ne sert qu'une fois).
func (m *MemoireCourte) Prendre(cle string) (any, bool) {
	v, ok := m.Lire(cle)
	m.Effacer(cle)
	return v, ok
}

// Effacer oublie une entrée.
func (m *MemoireCourte) Effacer(cle string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entrees, cle)
}
