// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"errors"
	"sync"
	"time"
)

// MemoireCourte : petites données qui ne vivent que quelques minutes
// (connexion en attente du second facteur, défi d'une passkey, secret TOTP
// pas encore confirmé). En mémoire seulement : un redémarrage les oublie,
// il suffit alors de recommencer. Bornée pour ne jamais grossir.
type MemoireCourte struct {
	mu      sync.Mutex
	entrees map[string]entreeCourte
	max     int
}

type entreeCourte struct {
	valeur any
	expire time.Time
}

// ErrMemoirePleine : trop d'opérations en cours (avalanche de requêtes).
var ErrMemoirePleine = errors.New("trop d'opérations en cours : réessayez dans une minute")

// NouvelleMemoireCourte : au plus max entrées.
func NouvelleMemoireCourte(max int) *MemoireCourte {
	return &MemoireCourte{entrees: map[string]entreeCourte{}, max: max}
}

// Mettre range une valeur pour la durée donnée.
func (m *MemoireCourte) Mettre(cle string, valeur any, duree time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := time.Now()
	if _, existe := m.entrees[cle]; !existe && len(m.entrees) >= m.max {
		for k, e := range m.entrees {
			if t.After(e.expire) {
				delete(m.entrees, k)
			}
		}
		if len(m.entrees) >= m.max {
			return ErrMemoirePleine
		}
	}
	m.entrees[cle] = entreeCourte{valeur: valeur, expire: t.Add(duree)}
	return nil
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
