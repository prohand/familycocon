// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"strings"
	"testing"
	"time"
)

// Une session utilisée chaque jour expire quand même après FC_SESSION_MAX_JOURS
// (un cookie volé ne sert pas indéfiniment) ; avant, elle est prolongée et le
// cookie du navigateur aussi.
func TestSessionDureeMax(t *testing.T) {
	a, _, marie := familleTest(t)
	if _, page := marie.get("/"); !strings.Contains(page, "Bonjour Marie") {
		t.Fatal("pas connectée")
	}
	// Vue il y a 2 h : prolongée, cookie renvoyé.
	if _, err := a.DB.Exec(`UPDATE sessions SET vue_le = ?`, time.Now().Add(-2*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	rep, err := marie.c.Get(marie.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	rep.Body.Close()
	if !strings.Contains(rep.Header.Get("Set-Cookie"), "fc_session") {
		t.Fatal("cookie de session non renouvelé")
	}
	// Connectée il y a plus d'un an, mais utilisée hier : refusée.
	ilYA := time.Now().Add(-366 * 24 * time.Hour).Unix()
	if _, err := a.DB.Exec(`UPDATE sessions SET cree_le = ?, vue_le = ?, expire_le = ?`, ilYA, time.Now().Add(-24*time.Hour).Unix(), time.Now().Add(80*24*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, page := marie.get("/"); strings.Contains(page, "Bonjour Marie") {
		t.Fatal("session de plus d'un an acceptée")
	}
	var n int
	_ = a.DB.QueryRow(`SELECT count(*) FROM sessions`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d session(s) restante(s)", n)
	}
}
