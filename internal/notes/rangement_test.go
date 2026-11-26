// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"testing"
	"time"
)

// Le texte du rappel compte les jours par la date : juste aussi autour du
// passage à l'heure d'été (le dimanche 29/03/2026 dure 23 h à Paris).
func TestQuandRappelChangementDHeure(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	rappel := time.Date(2026, 3, 30, 18, 0, 0, 0, paris)
	for maintenant, attendu := range map[time.Time]string{
		time.Date(2026, 3, 28, 10, 0, 0, 0, paris): "lundi 30/03 à 18:00",
		time.Date(2026, 3, 29, 10, 0, 0, 0, paris): "demain à 18:00",
		time.Date(2026, 3, 30, 8, 0, 0, 0, paris):  "aujourd'hui à 18:00",
	} {
		if q := quandRappel(rappel, maintenant); q != attendu {
			t.Errorf("le %s : %q, attendu %q", maintenant.Format("02/01"), q, attendu)
		}
	}
}
