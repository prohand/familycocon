// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import "testing"

func TestLireEmoji(t *testing.T) {
	for texte, attendu := range map[string]string{
		"🚗": "🚗", " ☁️ ": "☁️", "👥‍👧": "👥‍👧", "🏖️": "🏖️",
		"1️⃣": "1️⃣", "#️⃣": "#️⃣", "ℹ️": "ℹ️", "1": "🎁", "#": "🎁",
		"": "🎁", "a": "🎁", "<b>": "🎁", "🚗 auto": "🎁", "é": "🎁", "🚗🚗🚗🚗🚗🚗🚗🚗🚗": "🎁",
	} {
		if r := LireEmoji(texte, "🎁"); r != attendu {
			t.Errorf("LireEmoji(%q) = %q, attendu %q", texte, r, attendu)
		}
	}
}
