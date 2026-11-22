// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/webpush"
)

// Notifications Web Push : quand un membre fait quelque chose (ajoute un
// événement, valide une mission…), les autres membres qui n'ont pas
// l'application ouverte reçoivent une notification sur leur téléphone.
//   - la clé VAPID du serveur est créée au premier démarrage et gardée en
//     base (elle suit les sauvegardes) ;
//   - les envois partent d'une file en arrière-plan : une requête n'attend
//     jamais Google, Apple ou Mozilla ;
//   - un abonnement que le service dit expiré est supprimé.

// Pousseur : la file d'envoi des notifications.
type Pousseur struct {
	a       *App
	cle     *ecdsa.PrivateKey
	public  string
	file    chan annonce
	client  *http.Client
	envoyer func(ctx context.Context, ab *webpush.Abonnement, corps []byte, o webpush.Options) error
}

type annonce struct {
	famille, auteur int64
	module, texte   string
}

// tailleFile : au-delà, les annonces sont perdues (elles restent visibles
// dans l'application) : pas de mémoire qui gonfle si un service est lent.
const tailleFile = 64

// DemarrerPush charge (ou crée) la clé VAPID et lance la file d'envoi.
func (a *App) DemarrerPush(ctx context.Context) error {
	cle, err := a.cleVAPID(ctx)
	if err != nil {
		return err
	}
	public, err := webpush.ClePublique(cle)
	if err != nil {
		return err
	}
	p := &Pousseur{a: a, cle: cle, public: public, file: make(chan annonce, tailleFile),
		client: &http.Client{Timeout: 15 * time.Second}}
	p.envoyer = func(ctx context.Context, ab *webpush.Abonnement, corps []byte, o webpush.Options) error {
		return webpush.Envoyer(ctx, p.client, ab, corps, o)
	}
	a.Push = p
	go p.boucle(ctx)
	return nil
}

func (a *App) cleVAPID(ctx context.Context) (*ecdsa.PrivateKey, error) {
	brut, err := a.Q.LireServeur(ctx, "vapid")
	if errors.Is(err, sql.ErrNoRows) {
		cle, err := webpush.NouvelleCle()
		if err != nil {
			return nil, err
		}
		der, err := x509.MarshalECPrivateKey(cle)
		if err != nil {
			return nil, err
		}
		if err := a.Q.EcrireServeur(ctx, donnees.EcrireServeurParams{Nom: "vapid", Valeur: der}); err != nil {
			return nil, err
		}
		// Relue : si deux démarrages se croisent, tout le monde garde la même.
		if brut, err = a.Q.LireServeur(ctx, "vapid"); err != nil {
			return nil, err
		}
		a.Journal.Info("clé de notification (VAPID) créée")
	} else if err != nil {
		return nil, err
	}
	return x509.ParseECPrivateKey(brut)
}

// Annoncer met une notification dans la file (sans attendre).
func (p *Pousseur) Annoncer(famille, auteur int64, module, texte string) {
	select {
	case p.file <- annonce{famille: famille, auteur: auteur, module: module, texte: texte}:
	default:
		p.a.Journal.Warn("file des notifications pleine : notification perdue")
	}
}

func (p *Pousseur) boucle(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case an := <-p.file:
			p.diffuser(ctx, an)
		}
	}
}

// diffuser envoie l'annonce aux appareils de la famille concernés.
func (p *Pousseur) diffuser(ctx context.Context, an annonce) {
	abos, err := p.a.Q.AbonnementsFamille(ctx, donnees.AbonnementsFamilleParams{FamilleID: an.famille, Auteur: an.auteur})
	if err != nil {
		p.a.Journal.Error("notifications : lecture des abonnements", "erreur", err)
		return
	}
	url := "/"
	if m, ok := ModuleParCle(an.module); ok {
		url = m.URL
	}
	corps, _ := json.Marshal(map[string]string{"titre": "FamilyCocon", "texte": an.texte, "url": url, "groupe": an.module})
	for _, ab := range abos {
		if !contient(strings.Split(ab.PushModules, ","), an.module) || p.a.Direct.EnLigne(an.famille, ab.MembreID) {
			continue
		}
		envoi, annuler := context.WithTimeout(ctx, 20*time.Second)
		err := p.envoyer(envoi, &webpush.Abonnement{Adresse: ab.Adresse, P256dh: ab.P256dh, Auth: ab.Auth}, corps,
			webpush.Options{Cle: p.cle, Sujet: p.a.Config.ContactPush, Groupe: an.module, Urgent: an.module == "enfants"})
		annuler()
		switch {
		case errors.Is(err, webpush.ErrExpire), errors.Is(err, webpush.ErrAbonnement):
			_ = p.a.Q.SupprimerAbonnement(ctx, ab.ID)
			p.a.Journal.Info("abonnement push expiré, supprimé", "membre", ab.MembreID)
		case err != nil:
			p.a.Journal.Warn("notification non envoyée", "membre", ab.MembreID, "erreur", err)
		}
	}
}

// Routes : clé publique, abonnement et désabonnement de l'appareil.
func (a *App) routesPush() {
	a.Route("GET /push/cle", "", Membre, func(w http.ResponseWriter, r *http.Request, c *Contexte) {
		if a.Push == nil {
			http.Error(w, "notifications indisponibles", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(a.Push.public))
	})
	a.Route("POST /push/abonnement", "", Membre, a.abonnerPush)
	a.Route("POST /push/desabonnement", "", Membre, a.desabonnerPush)
	a.Route("POST /moi/notifications", "", Membre, a.choixNotifications)
}

// abonnementJS : l'abonnement tel que l'envoie le navigateur (PushSubscription.toJSON).
type abonnementJS struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (a *App) abonnerPush(w http.ResponseWriter, r *http.Request, c *Contexte) {
	var js abonnementJS
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&js); err != nil {
		http.Error(w, "abonnement illisible", http.StatusBadRequest)
		return
	}
	p256, err1 := base64.RawURLEncoding.DecodeString(strings.TrimRight(js.Keys.P256dh, "="))
	auth, err2 := base64.RawURLEncoding.DecodeString(strings.TrimRight(js.Keys.Auth, "="))
	ab := webpush.Abonnement{Adresse: js.Endpoint, P256dh: p256, Auth: auth}
	if err1 != nil || err2 != nil || ab.Valider() != nil {
		http.Error(w, "abonnement refusé", http.StatusBadRequest)
		return
	}
	if err := a.Q.Abonner(r.Context(), donnees.AbonnerParams{MembreID: c.Membre.ID, Adresse: ab.Adresse, P256dh: p256, Auth: auth, CreeLe: time.Now().Unix()}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Journal.Info("appareil abonné aux notifications", "membre", c.Membre.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) desabonnerPush(w http.ResponseWriter, r *http.Request, c *Contexte) {
	var js abonnementJS
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&js); err != nil {
		http.Error(w, "illisible", http.StatusBadRequest)
		return
	}
	if err := a.Q.Desabonner(r.Context(), donnees.DesabonnerParams{Adresse: js.Endpoint, MembreID: c.Membre.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// choixNotifications : modules qui notifient ce membre.
func (a *App) choixNotifications(w http.ResponseWriter, r *http.Request, c *Contexte) {
	if err := r.ParseForm(); err != nil {
		a.Echec(w, r, http.StatusBadRequest, "Formulaire invalide.")
		return
	}
	var choix []string
	for _, m := range Catalogue {
		if r.PostForm.Get("push_"+m.Cle) == "1" {
			choix = append(choix, m.Cle)
		}
	}
	if err := a.Q.ModifierPushModules(r.Context(), donnees.ModifierPushModulesParams{PushModules: strings.Join(choix, ","), ID: c.Membre.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Flash(w, "Notifications enregistrées ✓")
	a.Rediriger(w, r, "/moi")
}
