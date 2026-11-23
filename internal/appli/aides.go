// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"strconv"
	"sync"
	"time"

	"familycocon/internal/donnees"
)

// Petites aides communes aux modules (elles étaient recopiées dans chacun).

// MembresParID : les membres rangés par identifiant (les pointeurs visent
// la liste donnée : ne pas la modifier ensuite).
func MembresParID(membres []donnees.Membre) map[int64]*donnees.Membre {
	parID := make(map[int64]*donnees.Membre, len(membres))
	for i := range membres {
		parID[membres[i].ID] = &membres[i]
	}
	return parID
}

// TitreJour : « Lundi 16 ».
func TitreJour(t time.Time) string {
	j := JourFR(t)
	return string(j[0]-32) + j[1:] + " " + strconv.Itoa(t.Day())
}

// PeutModifier : un parent modifie tout ; les autres, ce qu'ils ont créé.
func PeutModifier(c *Contexte, creePar int64, connu bool) bool {
	return c.EstParent() || (connu && creePar == c.Membre.ID)
}

// MemoRequete garde le dernier résultat calculé pour une requête (un
// Contexte) : sur l'accueil, la tuile et le bloc d'un même module demandent
// la même chose l'un après l'autre, le calcul n'est fait qu'une fois.
// Une seule case : une autre requête la remplace (jamais de mémoire qui gonfle).
type MemoRequete[T any] struct {
	mu  sync.Mutex
	c   *Contexte
	v   T
	err error
}

// Lire renvoie le résultat déjà calculé pour ce Contexte, sinon appelle f.
func (m *MemoRequete[T]) Lire(c *Contexte, f func() (T, error)) (T, error) {
	m.mu.Lock()
	if m.c == c && c != nil {
		v, err := m.v, m.err
		m.mu.Unlock()
		return v, err
	}
	m.mu.Unlock()
	v, err := f()
	m.mu.Lock()
	m.c, m.v, m.err = c, v, err
	m.mu.Unlock()
	return v, err
}
