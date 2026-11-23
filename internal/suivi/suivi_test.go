// SPDX-License-Identifier: GPL-3.0-or-later

package suivi

import (
	"testing"
	"time"
)

func TestAjouterMois(t *testing.T) {
	cas := map[string]string{"2026-01-31+1": "2026-02-28", "2024-02-29+12": "2025-02-28", "2026-08-31+6": "2027-02-28", "2026-11-15+3": "2027-02-15"}
	for e, attendu := range cas {
		var j string
		var n int
		for i := range e {
			if e[i] == '+' {
				j = e[:i]
				for _, c := range e[i+1:] {
					n = n*10 + int(c-'0')
				}
			}
		}
		d, _ := time.Parse("2006-01-02", j)
		if g := AjouterMois(d, n).Format("2006-01-02"); g != attendu {
			t.Errorf("%s : %s, attendu %s", e, g, attendu)
		}
	}
}
