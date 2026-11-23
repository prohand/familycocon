// SPDX-License-Identifier: GPL-3.0-or-later

package webpush

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Le client d'envoi refuse de joindre une adresse interne.
func TestClientSur(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	if rep, err := ClientSur(2 * time.Second).Get(srv.URL); err == nil {
		rep.Body.Close()
		t.Fatal("adresse de bouclage jointe")
	}
}
