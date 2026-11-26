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
	"slices"
	"strings"
	"sync"
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
	parents         bool    // aux parents seulement
	sansParents     bool    // à tous sauf les parents (rappels : les parents ont leur propre texte)
	url             string  // page ouverte au toucher (sinon celle du module)
	pour            []int64 // membres visés (vide : toute la famille)
	sauf            []int64 // membres exclus (ils ont reçu leur propre version)
	toujours        bool    // même aux membres qui ont l'appli ouverte (rappel à une heure dite)
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
		client: webpush.ClientSur(15 * time.Second)}
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
	p.annoncer(annonce{famille: famille, auteur: auteur, module: module, texte: texte})
}

func (p *Pousseur) annoncer(an annonce) {
	select {
	case p.file <- an:
	default:
		p.a.Journal.Warn("file des notifications pleine : notification perdue")
	}
}

// AnnoncerParents : comme Annoncer, pour les parents seulement.
func (p *Pousseur) AnnoncerParents(famille int64, module, texte string) {
	select {
	case p.file <- annonce{famille: famille, module: module, texte: texte, parents: true}:
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
			func() {
				defer p.a.rattraper("notifications")
				p.diffuser(ctx, an)
			}()
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
		an.parents = an.parents || m.Parents
	}
	if an.url != "" {
		url = an.url
	}
	corps, _ := json.Marshal(map[string]string{"titre": "FamilyCocon", "texte": an.texte, "url": url, "groupe": an.module})
	// Heures calmes : une notification d'activité n'est pas envoyée pendant
	// celles du membre (elle reste visible dans l'appli). Les rappels à une
	// heure choisie et celui du matin partent quand même.
	calmePermis := !an.toujours && an.module != moduleRappels
	var heure time.Time // heure de la famille, lue au premier besoin
	calme := func(debut, fin string) bool {
		if !calmePermis || debut == "" || fin == "" {
			return false
		}
		if heure.IsZero() {
			heure = time.Now()
			if f, err := p.a.Q.FamilleParID(ctx, an.famille); err == nil {
				if l, err := time.LoadLocation(f.Fuseau); err == nil {
					heure = heure.In(l)
				}
			}
		}
		return EnHeuresCalmes(debut, fin, heure)
	}
	// Quelques envois à la fois : un service de push lent (20 s de délai)
	// ne retarde pas les autres appareils ni les annonces suivantes.
	var groupe sync.WaitGroup
	places := make(chan struct{}, envoisSimultanes)
	for _, ab := range abos {
		// Un rappel du matin ne s'affiche nulle part ailleurs : il part même si
		// le membre a une page ouverte (un onglet oublié sur l'ordinateur).
		enLigne := an.module != moduleRappels && !an.toujours && p.a.Direct.EnLigne(an.famille, ab.MembreID)
		if !contient(strings.Split(ab.PushModules, ","), an.module) || enLigne ||
			(an.parents && ab.Role != RoleParent) || (an.sansParents && ab.Role == RoleParent) ||
			(len(an.pour) > 0 && !slices.Contains(an.pour, ab.MembreID)) || slices.Contains(an.sauf, ab.MembreID) ||
			calme(ab.CalmeDebut, ab.CalmeFin) {
			continue
		}
		places <- struct{}{}
		groupe.Add(1)
		go func() {
			defer groupe.Done()
			defer func() { <-places }()
			defer p.a.rattraper("envoi d'une notification")
			envoi, annuler := context.WithTimeout(ctx, 20*time.Second)
			defer annuler()
			err := p.envoyer(envoi, &webpush.Abonnement{Adresse: ab.Adresse, P256dh: ab.P256dh, Auth: ab.Auth}, corps,
				webpush.Options{Cle: p.cle, Sujet: p.a.Config.ContactPush, Groupe: an.module, Urgent: an.module == "enfants"})
			switch {
			case errors.Is(err, webpush.ErrExpire), errors.Is(err, webpush.ErrAbonnement):
				_ = p.a.Q.SupprimerAbonnement(ctx, ab.ID)
				p.a.Journal.Info("abonnement push expiré, supprimé", "membre", ab.MembreID)
			case err != nil:
				p.a.Journal.Warn("notification non envoyée", "membre", ab.MembreID, "erreur", err)
			}
		}()
	}
	groupe.Wait()
}

// EnHeuresCalmes : t (heure de la famille) est-il dans la plage « debut »
// – « fin » (« 21:00 » – « 07:00 » : elle passe minuit) ? Plage vide ou
// invalide : non.
func EnHeuresCalmes(debut, fin string, t time.Time) bool {
	d, errD := time.Parse("15:04", debut)
	f, errF := time.Parse("15:04", fin)
	if errD != nil || errF != nil {
		return false
	}
	md, mf, m := d.Hour()*60+d.Minute(), f.Hour()*60+f.Minute(), t.Hour()*60+t.Minute()
	switch {
	case md == mf:
		return false
	case md < mf:
		return m >= md && m < mf
	}
	return m >= md || m < mf // la nuit : de 21:00 à minuit, puis jusqu'à 07:00
}

// heuresCalmes : POST /moi/calme (« de », « a » : HH:MM ; vides : aucune).
func (a *App) heuresCalmes(w http.ResponseWriter, r *http.Request, c *Contexte) {
	de, jusqua := r.PostFormValue("de"), r.PostFormValue("a")
	if r.PostFormValue("actives") != "1" {
		de, jusqua = "", ""
	}
	_, errD := time.Parse("15:04", de)
	_, errA := time.Parse("15:04", jusqua)
	if de != "" && (errD != nil || errA != nil || de == jusqua) {
		a.Echec(w, r, http.StatusBadRequest, "Heures calmes : deux heures différentes (par exemple de 21:00 à 07:00).")
		return
	}
	if err := a.Q.ModifierHeuresCalmes(r.Context(), donnees.ModifierHeuresCalmesParams{CalmeDebut: de, CalmeFin: jusqua,
		ID: c.Membre.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	if de == "" {
		a.Flash(w, "Heures calmes retirées")
	} else {
		a.Flash(w, "Heures calmes : de "+de+" à "+jusqua+" ✓")
	}
	a.Rediriger(w, r, "/moi")
}

// Rappeler : une notification à l'heure demandée (rappel d'une liste…),
// envoyée même aux membres qui ont l'appli ouverte. pour : membres visés
// (vide : toute la famille). Tâches de fond seulement (attend une place).
func (a *App) Rappeler(ctx context.Context, famille int64, module, texte, url string, pour []int64) {
	if a.Push == nil {
		return
	}
	a.Push.annoncerEnAttendant(ctx, annonce{famille: famille, module: module, texte: texte, url: url, pour: pour, toujours: true})
}

// envoisSimultanes : envois vers les services de push en même temps.
const envoisSimultanes = 4

// annoncerEnAttendant : comme Annoncer, mais attend une place dans la file
// (tâches de fond seulement : les rappels du matin de nombreuses familles
// ne doivent pas être perdus parce que la file est pleine).
func (p *Pousseur) annoncerEnAttendant(ctx context.Context, an annonce) {
	select {
	case p.file <- an:
	case <-ctx.Done():
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
	a.Route("POST /moi/calme", "", Membre, a.heuresCalmes)
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
	// 10 appareils au plus par membre : les plus anciens sont oubliés
	// (des milliers d'adresses ralentiraient les envois de tout le monde).
	if err := a.Q.GarderDerniersAbonnements(r.Context(), donnees.GarderDerniersAbonnementsParams{Membre: c.Membre.ID, Garder: 10}); err != nil {
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
	if r.PostForm.Get("push_"+moduleRappels) == "1" {
		choix = append(choix, moduleRappels)
	}
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
