// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"familycocon/internal/webpush"
)

// abonnementFactice : un abonnement valide (clés réelles, service FCM).
func abonnementFactice(t *testing.T, fin string) string {
	t.Helper()
	cle, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	b, _ := json.Marshal(map[string]any{
		"endpoint": "https://fcm.googleapis.com/fcm/send/" + fin,
		"keys": map[string]string{
			"p256dh": base64.RawURLEncoding.EncodeToString(cle.PublicKey().Bytes()),
			"auth":   base64.RawURLEncoding.EncodeToString(auth),
		},
	})
	return string(b)
}

func TestNotificationsPush(t *testing.T) {
	a, srv, marie := familleTest(t)
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	if err := a.DemarrerPush(ctx); err != nil {
		t.Fatal(err)
	}
	// La clé est gardée : un second démarrage relit la même.
	public := a.Push.public
	if err := a.DemarrerPush(ctx); err != nil || a.Push.public != public {
		t.Fatal("clé VAPID recréée au redémarrage")
	}

	var mu sync.Mutex
	var envois []string
	expire := false
	a.Push.envoyer = func(_ context.Context, ab *webpush.Abonnement, corps []byte, o webpush.Options) error {
		mu.Lock()
		defer mu.Unlock()
		envois = append(envois, ab.Adresse+" "+string(corps))
		if expire {
			return webpush.ErrExpire
		}
		return nil
	}

	// Thomas rejoint la famille et abonne son téléphone.
	marie.get("/famille")
	_, page := marie.post("/famille/invitations", map[string]string{"role": "parent"})
	lien := page[strings.Index(page, "/invitation/"):][:len("/invitation/")+43]
	thomas := nouveauNavigateur(t, srv)
	thomas.post(lien, map[string]string{"prenom": "Thomas", "identifiant": "thomas", "mdp": "le papa du nid", "mdp2": "le papa du nid"})
	thomas.get("/moi")
	req, _ := http.NewRequest("POST", srv.URL+"/push/abonnement", strings.NewReader(abonnementFactice(t, "thomas")))
	req.Header.Set("X-Jeton", thomas.jeton)
	rep, err := thomas.c.Do(req)
	if err != nil || rep.StatusCode != http.StatusNoContent {
		t.Fatalf("abonnement : %v %v", rep.StatusCode, err)
	}
	// Abonnement refusé : service inconnu.
	req, _ = http.NewRequest("POST", srv.URL+"/push/abonnement", strings.NewReader(strings.Replace(abonnementFactice(t, "x"), "fcm.googleapis.com", "pirate.example", 1)))
	req.Header.Set("X-Jeton", thomas.jeton)
	if rep, _ := thomas.c.Do(req); rep.StatusCode != http.StatusBadRequest {
		t.Fatalf("service inconnu accepté : %d", rep.StatusCode)
	}

	attendre := func(n int) []string {
		for i := 0; i < 200; i++ {
			mu.Lock()
			l := append([]string(nil), envois...)
			mu.Unlock()
			if len(l) >= n {
				return l
			}
			time.Sleep(10 * time.Millisecond)
		}
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), envois...)
	}
	cMarie := contexteDe(t, a, "marie")

	// Module « courses » : pas dans les choix par défaut → rien.
	a.Publier(cMarie, "courses", "Marie a ajouté « Pain »")
	// Module « agenda » : envoyé à Thomas (pas à Marie, l'autrice).
	a.Publier(cMarie, "agenda", "Marie a ajouté « Piscine »")
	l := attendre(1)
	time.Sleep(50 * time.Millisecond)
	l = attendre(1)
	if len(l) != 1 || !strings.Contains(l[0], "fcm/send/thomas") || !strings.Contains(l[0], "Piscine") || !strings.Contains(l[0], `"url":"/agenda"`) {
		t.Fatalf("envois : %v", l)
	}

	// Thomas a une page ouverte : pas de push.
	ch := a.Direct.abonner(1, membreID(t, a, "thomas"))
	a.Publier(cMarie, "agenda", "Marie a ajouté « Dentiste »")
	time.Sleep(100 * time.Millisecond)
	if len(attendre(1)) != 1 {
		t.Fatal("push envoyé alors que la page est ouverte")
	}
	a.Direct.desabonner(1, ch)

	// Abonnement expiré : supprimé.
	mu.Lock()
	expire = true
	mu.Unlock()
	a.Publier(cMarie, "taches", "Marie a ajouté une tâche")
	attendre(2)
	for i := 0; i < 100; i++ {
		if n, _ := a.Q.CompterAbonnements(context.Background(), membreID(t, a, "thomas")); n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("abonnement expiré non supprimé")
}

func membreID(t *testing.T, a *App, ident string) int64 {
	t.Helper()
	m, err := a.Q.MembreParIdentifiant(context.Background(), sqlTexte(ident))
	if err != nil {
		t.Fatal(err)
	}
	return m.ID
}

func contexteDe(t *testing.T, a *App, ident string) *Contexte {
	t.Helper()
	m, err := a.Q.MembreParIdentifiant(context.Background(), sqlTexte(ident))
	if err != nil {
		t.Fatal(err)
	}
	f, err := a.Q.FamilleParID(context.Background(), m.FamilleID)
	if err != nil {
		t.Fatal(err)
	}
	return a.NouveauContexte(m, f, "", nil)
}

func sqlTexte(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
