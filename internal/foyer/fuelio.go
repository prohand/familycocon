// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/fuelio"
	"familycocon/internal/photo"
	"familycocon/internal/suivi"
	txt "familycocon/internal/texte"
)

// Reprise d'une sauvegarde Fuelio dans le carnet d'un véhicule (parents) :
// les pleins (litres, prix, kilométrage) et les dépenses, avec leurs photos
// (tickets, factures) quand la sauvegarde en contient.
//   - le fichier est écrit sur le disque, à côté de la base, le temps de
//     la lecture (jamais en entier en mémoire), puis effacé ;
//   - une ligne déjà présente (même jour, libellé, montant, kilométrage)
//     n'est pas ajoutée deux fois : on peut reprendre la sauvegarde
//     suivante sans doublons ;
//   - rien n'est ajouté au budget (ce sont des dépenses passées).

// fuelioUn : une reprise à la fois (lecture des photos gourmande en mémoire).
var fuelioUn sync.Mutex

// fuelioRetour : ce que la reprise a fait.
type fuelioRetour struct {
	pleins, depenses, deja, photos, sansLigne int  // sansLigne : photos non jointes
	corriges                                  int  // pleins déjà là, devenus partiels (ou complets)
	plein                                     bool // carnet plein : la fin n'est pas reprise
	ecart                                     fuelio.Ecart
}

func (x fuelioRetour) message() string {
	var l []string
	if x.pleins > 0 {
		l = append(l, appli.Pluriel(x.pleins, "plein", "pleins"))
	}
	if x.depenses > 0 {
		l = append(l, appli.Pluriel(x.depenses, "dépense", "dépenses"))
	}
	t := "Fuelio : rien de nouveau"
	if len(l) > 0 {
		t = "Fuelio : " + strings.Join(l, " et ") + " repris"
	}
	if x.deja > 0 {
		t += fmt.Sprintf(" · %d déjà là", x.deja)
	}
	if x.corriges > 0 {
		t += " · " + appli.Pluriel(x.corriges, "plein corrigé (partiel)", "pleins corrigés (partiels)")
	}
	if x.photos > 0 {
		t += " · " + appli.Pluriel(x.photos, "photo jointe", "photos jointes")
	}
	if x.sansLigne > 0 {
		t += " · " + appli.Pluriel(x.sansLigne, "photo laissée de côté", "photos laissées de côté")
	}
	if x.plein {
		t += " · carnet plein"
	}
	// Ce qui n'a pas été repris, pour que l'oubli ne passe pas inaperçu.
	e := x.ecart
	if e.DepensesLues == 0 {
		t += " · aucune dépense (entretien) dans cette sauvegarde"
	}
	var ec []string
	if n := e.PleinsSansDate + e.DepensesSansDate; n > 0 {
		ec = append(ec, fmt.Sprintf("%d sans date lisible", n))
	}
	if e.Modeles > 0 {
		ec = append(ec, appli.Pluriel(e.Modeles, "modèle de dépense répétée", "modèles de dépenses répétées"))
	}
	if e.Recettes > 0 {
		ec = append(ec, appli.Pluriel(e.Recettes, "recette", "recettes"))
	}
	if len(ec) > 0 {
		t += " · écartés : " + strings.Join(ec, ", ")
	}
	return t
}

// recevoirFuelio : le fichier envoyé (corps brut, voir app.js), écrit sur
// le disque puis ouvert. fin : à appeler quand tout est fini (ferme, efface
// le fichier provisoire, libère la place). ok faux : réponse déjà faite.
func (m *Module) recevoirFuelio(w http.ResponseWriter, r *http.Request) (sv *fuelio.Sauvegarde, fin func(), ok bool) {
	if !fuelioUn.TryLock() {
		m.a.Echec(w, r, http.StatusServiceUnavailable, "Une reprise Fuelio est déjà en cours : réessayez dans un instant.")
		return nil, nil, false
	}
	// Le fichier va sur le disque (dossier des données), jamais en mémoire.
	tmp, err := os.CreateTemp(m.a.Config.Donnees, "fuelio-*.tmp")
	if err != nil {
		fuelioUn.Unlock()
		m.a.ErreurInterne(w, r, err)
		return nil, nil, false
	}
	nettoyer := func() { _ = os.Remove(tmp.Name()); fuelioUn.Unlock() }
	_, err = io.Copy(tmp, r.Body) // borné par le serveur (appli.TailleMaxFuelio)
	if errFermer := tmp.Close(); err == nil {
		err = errFermer
	}
	var trop *http.MaxBytesError
	if errors.As(err, &trop) {
		nettoyer()
		m.a.Echec(w, r, http.StatusRequestEntityTooLarge, fmt.Sprintf("Fichier trop gros (%d Mo au plus).", appli.TailleMaxFuelio>>20))
		return nil, nil, false
	}
	if err != nil {
		nettoyer()
		m.a.Echec(w, r, http.StatusBadRequest, "Envoi interrompu : réessayez.")
		return nil, nil, false
	}
	sv, err = fuelio.Ouvrir(tmp.Name())
	if err != nil {
		nettoyer()
		m.a.Echec(w, r, http.StatusBadRequest, "Ce fichier n'est pas une sauvegarde Fuelio (.fuelio, .zip ou .csv).")
		return nil, nil, false
	}
	return sv, func() { _ = sv.Fermer(); nettoyer() }, true
}

// memeNom : deux noms de véhicule pareils (accents et majuscules à part).
func memeNom(a, b string) bool {
	return txt.Plier(strings.TrimSpace(a)) == txt.Plier(strings.TrimSpace(b))
}

// importerFuelio : POST du fichier sur la fiche d'un véhicule.
func (m *Module) importerFuelio(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.vehicule(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Véhicule introuvable.")
		return
	}
	sv, fin, ok := m.recevoirFuelio(w, r)
	if !ok {
		return
	}
	defer fin()
	// Plusieurs véhicules dans la sauvegarde : celui qui porte le même nom.
	v := sv.Vehicules[0]
	if len(sv.Vehicules) > 1 {
		trouve := false
		var noms []string
		for _, y := range sv.Vehicules {
			noms = append(noms, "« "+y.Nom+" »")
			if !trouve && memeNom(y.Nom, x.Nom) {
				v, trouve = y, true
			}
		}
		if !trouve {
			m.a.Echec(w, r, http.StatusBadRequest, "Cette sauvegarde contient plusieurs véhicules ("+strings.Join(noms, ", ")+
				") : donnez ici le même nom que dans Fuelio, ou reprenez-les tous depuis la liste des véhicules.")
			return
		}
	}
	ret, err := m.reprendre(r.Context(), c, x, v, sv.Images)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, modVehicules, "")
	m.a.Flash(w, ret.message())
	m.a.Rediriger(w, r, fmt.Sprintf("/vehicules/%d", x.ID))
}

// importerFuelioTous : POST du fichier sur la liste des véhicules : chaque
// véhicule de la sauvegarde va dans celui du même nom, créé s'il manque
// (immatriculation, marque, modèle et année de Fuelio). Un véhicule créé
// dont le dernier plein a plus d'un an est rangé dans les anciens (depuis
// ce plein) : il se corrige sur sa fiche.
func (m *Module) importerFuelioTous(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	sv, fin, ok := m.recevoirFuelio(w, r)
	if !ok {
		return
	}
	defer fin()
	existants, err := m.a.Q.VehiculesFamille(ctx, c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	var bilan []string
	for _, v := range sv.Vehicules {
		var x donnees.Vehicule
		trouve := false
		for _, e := range existants {
			if memeNom(e.Nom, v.Nom) {
				x, trouve = e, true
				break
			}
		}
		nom := strings.TrimSpace(v.Nom)
		if nom == "" {
			nom = "Véhicule Fuelio"
		}
		cree := false
		if !trouve {
			if len(existants) >= ObjetsMax {
				bilan = append(bilan, nom+" : pas créé (trop de véhicules)")
				continue
			}
			x, err = m.a.Q.CreerVehicule(ctx, donnees.CreerVehiculeParams{FamilleID: c.Famille.ID, Nom: couper(nom, 60), Genre: "voiture",
				Immatriculation: strings.ToUpper(v.Plaque), Modele: v.Modele, CreeLe: time.Now().Unix()})
			if err != nil {
				m.a.ErreurInterne(w, r, err)
				return
			}
			existants = append(existants, x)
			cree = true
		}
		ret, err := m.reprendre(ctx, c, x, v, sv.Images)
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		ligne := x.Nom
		if cree {
			ligne += " (ajouté"
			// Dernier plein il y a plus d'un an : un ancien véhicule.
			dernier := ""
			for _, l := range v.Lignes {
				if l.Jour > dernier {
					dernier = l.Jour
				}
			}
			if dernier != "" && dernier < c.Maintenant().AddDate(-1, 0, 0).Format("2006-01-02") {
				if err := m.a.Q.ChangerVenduVehicule(ctx, donnees.ChangerVenduVehiculeParams{VenduLe: dernier, ID: x.ID, FamilleID: c.Famille.ID}); err != nil {
					m.a.ErreurInterne(w, r, err)
					return
				}
				ligne += ", rangé dans les anciens"
			}
			ligne += ")"
		}
		bilan = append(bilan, ligne+" : "+strings.TrimPrefix(ret.message(), "Fuelio : "))
	}
	m.a.Publier(c, modVehicules, "")
	m.a.Flash(w, "Fuelio · "+strings.Join(bilan, " — "))
	m.a.Rediriger(w, r, "/vehicules")
}

// couper : au plus n caractères.
func couper(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// reprendre : les lignes d'un véhicule Fuelio dans le carnet du véhicule x
// (sans doublon), puis leurs photos, puis le compteur.
func (m *Module) reprendre(ctx context.Context, c *appli.Contexte, x donnees.Vehicule, v fuelio.Vehicule, images []fuelio.Image) (fuelioRetour, error) {
	ret := fuelioRetour{ecart: v.Ecart}
	n, err := m.a.Q.CompterCarnetObjet(ctx, donnees.CompterCarnetObjetParams{FamilleID: c.Famille.ID, Module: modVehicules, ObjetID: x.ID})
	if err != nil {
		return ret, err
	}
	ids := make([]int64, len(v.Lignes)) // ligne du carnet créée pour chaque ligne Fuelio (0 : aucune)
	var kmMax int64
	auteur := sql.NullInt64{Int64: c.Membre.ID, Valid: true}
	maintenant := time.Now().Unix()
	err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
		for i, l := range v.Lignes {
			deja, err := q.CarnetExiste(ctx, donnees.CarnetExisteParams{FamilleID: c.Famille.ID, Module: modVehicules, ObjetID: x.ID,
				Jour: l.Jour, Libelle: l.Libelle, Montant: l.Montant, Km: l.Km})
			if err != nil {
				return err
			}
			if deja > 0 {
				ret.deja++
				continue
			}
			// Plein repris avant sous l'autre libellé (complet / partiel) :
			// corrigé sur place.
			if l.Plein {
				ancien := map[string]string{"Plein": "Plein partiel", "Plein partiel": "Plein"}[l.Libelle]
				k, err := q.RenommerPleinImporte(ctx, donnees.RenommerPleinImporteParams{Nouveau: l.Libelle, FamilleID: c.Famille.ID,
					Module: modVehicules, ObjetID: x.ID, Jour: l.Jour, Ancien: ancien, Montant: l.Montant, Km: l.Km, Litres: l.Litres})
				if err != nil {
					return err
				}
				if k > 0 {
					ret.corriges++
					continue
				}
			}
			if n >= suivi.CarnetMax {
				ret.plein = true
				break
			}
			ligne, err := q.AjouterCarnet(ctx, donnees.AjouterCarnetParams{FamilleID: c.Famille.ID, Module: modVehicules, ObjetID: x.ID,
				Jour: l.Jour, Libelle: l.Libelle, Montant: l.Montant, Km: l.Km, Litres: l.Litres, Note: l.Note,
				CreePar: auteur, CreeLe: maintenant})
			if err != nil {
				return err
			}
			n++
			ids[i] = ligne.ID
			kmMax = max(kmMax, l.Km)
			if l.Plein {
				ret.pleins++
			} else {
				ret.depenses++
			}
		}
		return nil
	})
	if err != nil {
		return ret, err
	}

	// Les photos, une à une et hors transaction (la base reste libre
	// pendant ce temps). Seules les lignes ajoutées maintenant en reçoivent :
	// reprendre deux fois la même sauvegarde ne double pas les photos. La
	// suite va jusqu'au bout même si la page est quittée (téléphone mis en
	// veille) : les lignes sont déjà écrites, leurs photos ne doivent pas
	// manquer.
	ctx = context.WithoutCancel(ctx)
	liens, sansLigne := fuelio.Rattacher(v, images)
	ret.sansLigne = sansLigne
	for _, l := range liens {
		if ids[l.Ligne] == 0 {
			continue // ligne déjà là avant : ses photos aussi
		}
		jpeg, err := photo.PreparerFlux(l.Image.Ouvrir)
		if err == nil {
			err = suivi.AjouterPiece(ctx, m.a, m.a.Q, c.Famille.ID, ids[l.Ligne], c.Membre.ID, jpeg)
		}
		if err != nil {
			m.a.Journal.Warn("fuelio : photo non gardée", "fichier", l.Image.Nom, "erreur", err)
			ret.sansLigne++
			continue
		}
		ret.photos++
	}
	// Les photos décodées ont pris de la mémoire : rendue au système tout de
	// suite (sinon le serveur la garde plusieurs minutes).
	debug.FreeOSMemory()
	if kmMax > 0 {
		if err := m.a.Q.NoterKilometrage(ctx, donnees.NoterKilometrageParams{Kilometrage: kmMax, ID: x.ID, FamilleID: c.Famille.ID, Kilometrage_2: kmMax}); err != nil {
			m.a.Journal.Error("fuelio : kilométrage non noté", "erreur", err)
		}
	}
	m.a.Journal.Info("reprise Fuelio", "vehicule", x.ID, "pleins", ret.pleins, "depenses", ret.depenses, "deja", ret.deja, "corriges", ret.corriges,
		"photos", ret.photos, "photos_sans_ligne", ret.sansLigne, "images_zip", len(images),
		"depenses_lues", v.Ecart.DepensesLues, "modeles", v.Ecart.Modeles, "recettes", v.Ecart.Recettes,
		"sans_date", v.Ecart.PleinsSansDate+v.Ecart.DepensesSansDate, "nom", v.Nom)
	return ret, nil
}
