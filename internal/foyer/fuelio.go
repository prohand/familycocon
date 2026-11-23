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
	plein                                     bool // carnet plein : la fin n'est pas reprise
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
	if x.photos > 0 {
		t += " · " + appli.Pluriel(x.photos, "photo jointe", "photos jointes")
	}
	if x.sansLigne > 0 {
		t += " · " + appli.Pluriel(x.sansLigne, "photo laissée de côté", "photos laissées de côté")
	}
	if x.plein {
		t += " · carnet plein"
	}
	return t
}

// importerFuelio : POST du fichier, envoyé tel quel (corps brut, voir app.js).
func (m *Module) importerFuelio(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	x, ok := m.vehicule(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Véhicule introuvable.")
		return
	}
	if !fuelioUn.TryLock() {
		m.a.Echec(w, r, http.StatusServiceUnavailable, "Une reprise Fuelio est déjà en cours : réessayez dans un instant.")
		return
	}
	defer fuelioUn.Unlock()

	// Le fichier va sur le disque (dossier des données), jamais en mémoire.
	tmp, err := os.CreateTemp(m.a.Config.Donnees, "fuelio-*.tmp")
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, r.Body) // borné par le serveur (appli.TailleMaxFuelio)
	if errFermer := tmp.Close(); err == nil {
		err = errFermer
	}
	var trop *http.MaxBytesError
	if errors.As(err, &trop) {
		m.a.Echec(w, r, http.StatusRequestEntityTooLarge, fmt.Sprintf("Fichier trop gros (%d Mo au plus).", appli.TailleMaxFuelio>>20))
		return
	}
	if err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, "Envoi interrompu : réessayez.")
		return
	}
	sv, err := fuelio.Ouvrir(tmp.Name())
	if err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, "Ce fichier n'est pas une sauvegarde Fuelio (.fuelio, .zip ou .csv).")
		return
	}
	defer sv.Fermer()

	// Plusieurs véhicules dans la sauvegarde : celui qui porte le même nom.
	v := sv.Vehicules[0]
	if len(sv.Vehicules) > 1 {
		trouve := false
		var noms []string
		for _, y := range sv.Vehicules {
			noms = append(noms, "« "+y.Nom+" »")
			if !trouve && txt.Plier(strings.TrimSpace(y.Nom)) == txt.Plier(strings.TrimSpace(x.Nom)) {
				v, trouve = y, true
			}
		}
		if !trouve {
			m.a.Echec(w, r, http.StatusBadRequest, "Cette sauvegarde contient plusieurs véhicules ("+strings.Join(noms, ", ")+
				") : donnez ici le même nom que dans Fuelio, puis recommencez.")
			return
		}
	}

	n, err := m.a.Q.CompterCarnetObjet(ctx, donnees.CompterCarnetObjetParams{FamilleID: c.Famille.ID, Module: modVehicules, ObjetID: x.ID})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	var ret fuelioRetour
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
		m.a.ErreurInterne(w, r, err)
		return
	}

	// Les photos, une à une et hors transaction (la base reste libre
	// pendant ce temps). Seules les lignes ajoutées maintenant en reçoivent :
	// reprendre deux fois la même sauvegarde ne double pas les photos. La
	// suite va jusqu'au bout même si la page est quittée (téléphone mis en
	// veille) : les lignes sont déjà écrites, leurs photos ne doivent pas
	// manquer.
	ctx = context.WithoutCancel(ctx)
	liens, sansLigne := fuelio.Rattacher(v, sv.Images)
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
	m.a.Journal.Info("reprise Fuelio", "vehicule", x.ID, "pleins", ret.pleins, "depenses", ret.depenses, "deja", ret.deja,
		"photos", ret.photos, "photos_sans_ligne", ret.sansLigne, "images_zip", len(sv.Images))
	m.a.Publier(c, modVehicules, "")
	m.a.Flash(w, ret.message())
	m.a.Rediriger(w, r, fmt.Sprintf("/vehicules/%d", x.ID))
}
