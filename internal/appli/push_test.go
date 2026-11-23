// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"familycocon/internal/donnees"
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

	// Notification pour Marie seulement : rien pour Thomas.
	a.PublierPour(cMarie, "agenda", "Pour Marie seulement", []int64{membreID(t, a, "marie")})
	time.Sleep(100 * time.Millisecond)
	if l := attendre(1); len(l) != 1 {
		t.Fatalf("notification ciblée envoyée à un autre membre : %v", l)
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

func TestRappelsDuMatin(t *testing.T) {
	a, srv, marie := familleTest(t)
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	if err := a.DemarrerPush(ctx); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var recus []string
	a.Push.envoyer = func(_ context.Context, _ *webpush.Abonnement, corps []byte, _ webpush.Options) error {
		mu.Lock()
		defer mu.Unlock()
		recus = append(recus, string(corps))
		return nil
	}
	// Une échéance aujourd'hui, une dans 7 jours, une dans 3 jours (pas de rappel).
	a.AjouterEcheances("courses", func(_ context.Context, f donnees.Famille, debut, fin time.Time) ([]Echeance, error) {
		var out []Echeance
		auj := minuitDe(time.Now().In(lieuFamille(f)))
		for _, n := range []int{0, 3, 7} {
			j := auj.AddDate(0, 0, n)
			if !j.Before(debut) && j.Before(fin) {
				out = append(out, Echeance{Jour: j, Titre: fmt.Sprintf("Échéance J+%d", n), Emoji: "⏰", Module: "courses"})
			}
		}
		return out, nil
	})
	marie.get("/moi")
	req, _ := http.NewRequest("POST", srv.URL+"/push/abonnement", strings.NewReader(abonnementFactice(t, "marie")))
	req.Header.Set("X-Jeton", marie.jeton)
	if rep, err := marie.c.Do(req); err != nil || rep.StatusCode != http.StatusNoContent {
		t.Fatal("abonnement")
	}
	// Heure de la famille (Europe/Paris par défaut) : seulement à 9 h.
	paris, _ := time.LoadLocation("Europe/Paris")
	maintenant := time.Now().In(paris)
	a9h := time.Date(maintenant.Year(), maintenant.Month(), maintenant.Day(), 9, 0, 0, 0, paris)
	if n := a.envoyerRappelsA(ctx, a9h.Add(-time.Hour), true); n != 0 {
		t.Fatalf("%d rappels à 8 h, aucun attendu", n)
	}
	if n := a.envoyerRappelsA(ctx, a9h.Add(12*time.Hour), true); n != 0 {
		t.Fatalf("%d rappels à 21 h, aucun attendu (trop tard)", n)
	}
	// Serveur arrêté à 9 h, revenu à 10 h : les rappels partent quand même…
	if n := a.envoyerRappelsA(ctx, a9h.Add(time.Hour), true); n != 2 {
		t.Fatalf("%d rappels à 10 h (rattrapage), 2 attendus", n)
	}
	// … mais une seule fois par jour.
	if n := a.envoyerRappelsA(ctx, a9h.Add(2*time.Hour), true); n != 0 {
		t.Fatalf("%d rappels à 11 h, aucun attendu (déjà envoyés)", n)
	}
	for i := 0; i < 200; i++ {
		mu.Lock()
		l := len(recus)
		mu.Unlock()
		if l >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // une éventuelle deuxième notification
	mu.Lock()
	defer mu.Unlock()
	tout := strings.Join(recus, "\n")
	// Une seule notification pour les deux rappels (sinon la seconde
	// remplace la première à l'écran).
	if len(recus) != 1 || !strings.Contains(tout, "Aujourd'hui : Échéance J+0") || !strings.Contains(tout, "Dans 7 jours : Échéance J+7") {
		t.Fatalf("rappels reçus : %v", recus)
	}
	// Le bloc « À venir » de l'accueil les montre (30 jours).
	if _, page := marie.get("/"); !strings.Contains(page, "À venir") || !strings.Contains(page, "Échéance J&#43;3") {
		t.Fatal("bloc À venir")
	}
}

// Moi › Notifications : seuls les modules qui envoient des notifications
// ont un interrupteur (les autres ne servaient à rien).
func TestChoixNotificationsUtiles(t *testing.T) {
	_, _, marie := familleTest(t)
	_, page := marie.get("/moi")
	if !strings.Contains(page, `name="push_courses"`) || !strings.Contains(page, `name="push_rappels"`) {
		t.Fatal("interrupteurs des modules qui notifient absents")
	}
	if strings.Contains(page, `name="push_budget"`) || strings.Contains(page, `name="push_contacts"`) {
		t.Fatal("interrupteur d'un module qui ne notifie jamais")
	}
}

// Rappels : un seul rappel ouvre sa page ; plusieurs ouvrent l'accueil.
func TestTexteRappels(t *testing.T) {
	e := Echeance{Titre: "Vaccin", URL: "/animaux/3"}
	if texte, url := texteRappels([]Echeance{e}, []string{"💉 Aujourd'hui : Vaccin"}); url != "/animaux/3" || texte != "💉 Aujourd'hui : Vaccin" {
		t.Fatalf("un rappel : %q %q", texte, url)
	}
	if texte, url := texteRappels([]Echeance{e, e}, []string{"a", "b"}); url != "/" || texte != "a\nb" {
		t.Fatalf("deux rappels : %q %q", texte, url)
	}
}

// Temps réel : le texte d'une notification ciblée ne s'affiche que chez les
// membres visés ; les autres pages se mettent quand même à jour.
func TestDirectCible(t *testing.T) {
	d := nouveauDirect()
	vise, autre := d.abonner(1, 10), d.abonner(1, 20)
	d.PublierPour(1, Message{Module: "enfants", Texte: "à valider"}, []int64{10})
	if m := string(<-vise); !strings.Contains(m, "à valider") {
		t.Fatalf("membre visé : %s", m)
	}
	if m := string(<-autre); strings.Contains(m, "à valider") || !strings.Contains(m, `"module":"enfants"`) {
		t.Fatalf("autre membre : %s", m)
	}
}

// Carte « Serveur » : la dernière sauvegarde (réussie ou non) est montrée
// aux parents d'un serveur d'une seule famille, et à la supervision.
func TestCarteServeur(t *testing.T) {
	a, srv, marie := familleTest(t)
	if _, page := marie.get("/famille"); !strings.Contains(page, "🖥️ Serveur") || !strings.Contains(page, "pas encore") {
		t.Fatal("carte Serveur absente")
	}
	a.noterSauvegarde(context.Background(), EtatSauvegarde{Le: time.Now().Unix(), Erreur: "la sauvegarde a échoué"})
	if _, page := marie.get("/famille"); !strings.Contains(page, "❌") || !strings.Contains(page, "la sauvegarde a échoué") {
		t.Fatal("échec de la sauvegarde non montré")
	}
	rep, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	var s Sante
	_ = json.NewDecoder(rep.Body).Decode(&s)
	if s.Sauvegarde == "" || s.SauvegardeOK == nil || *s.SauvegardeOK {
		t.Fatalf("/health : %+v", s)
	}
	if Octets(2516582) != "2,4 Mo" || Octets(2048) != "2 Ko" {
		t.Fatal(Octets(2516582))
	}
}

// Échéance privée (abonnement ou projet privé) : seul son propriétaire la
// voit et la reçoit, dans sa propre notification du matin ; l'autre parent
// reçoit la notification commune, sans elle.
func TestRappelsPrives(t *testing.T) {
	a, srv, marie := familleTest(t)
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	if err := a.DemarrerPush(ctx); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	recus := map[string][]string{}
	a.Push.envoyer = func(_ context.Context, ab *webpush.Abonnement, corps []byte, _ webpush.Options) error {
		mu.Lock()
		defer mu.Unlock()
		qui := ab.Adresse[strings.LastIndex(ab.Adresse, "/")+1:]
		recus[qui] = append(recus[qui], string(corps))
		return nil
	}
	a.AjouterEcheances("courses", func(_ context.Context, f donnees.Famille, debut, fin time.Time) ([]Echeance, error) {
		auj := minuitDe(time.Now().In(lieuFamille(f)))
		if auj.Before(debut) || !auj.Before(fin) {
			return nil, nil
		}
		return []Echeance{{Jour: auj, Titre: "Pour tous", Emoji: "⏰", Module: "courses"},
			{Jour: auj, Titre: "Secret de Marie", Emoji: "🔒", Module: "courses", Pour: 1}}, nil
	})
	abonner := func(n *navigateur, nom string) {
		n.get("/moi")
		req, _ := http.NewRequest("POST", srv.URL+"/push/abonnement", strings.NewReader(abonnementFactice(t, nom)))
		req.Header.Set("X-Jeton", n.jeton)
		if rep, err := n.c.Do(req); err != nil || rep.StatusCode != http.StatusNoContent {
			t.Fatal("abonnement")
		}
	}
	marie.get("/famille")
	_, page := marie.post("/famille/invitations", map[string]string{"role": "parent"})
	lien := page[strings.Index(page, "/invitation/"):][:len("/invitation/")+43]
	paul := nouveauNavigateur(t, srv)
	paul.post(lien, map[string]string{"prenom": "Paul", "identifiant": "paul", "mdp": "le papa du nid", "mdp2": "le papa du nid"})
	abonner(marie, "marie")
	abonner(paul, "paul")
	paris, _ := time.LoadLocation("Europe/Paris")
	maintenant := time.Now().In(paris)
	a9h := time.Date(maintenant.Year(), maintenant.Month(), maintenant.Day(), 9, 30, 0, 0, paris)
	a.envoyerRappelsA(ctx, a9h, true)
	for i := 0; i < 200; i++ {
		mu.Lock()
		n := len(recus["marie"]) + len(recus["paul"])
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(recus["marie"]) != 1 || !strings.Contains(recus["marie"][0], "Secret de Marie") || !strings.Contains(recus["marie"][0], "Pour tous") {
		t.Fatalf("Marie : %v", recus["marie"])
	}
	if len(recus["paul"]) != 1 || strings.Contains(recus["paul"][0], "Secret") || !strings.Contains(recus["paul"][0], "Pour tous") {
		t.Fatalf("Paul : %v", recus["paul"])
	}
	// « À venir » sur l'accueil : pareil.
	if _, p := paul.get("/"); strings.Contains(p, "Secret de Marie") {
		t.Fatal("échéance privée sur l'accueil de Paul")
	}
	if _, p := marie.get("/"); !strings.Contains(p, "Secret de Marie") {
		t.Fatal("échéance privée absente de l'accueil de Marie")
	}
}
