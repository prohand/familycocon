// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/version"
)

// Export de la famille (Ma famille › Exporter) : toutes ses données dans
// un fichier JSON, pour les garder ou les emporter ailleurs (sur un
// serveur partagé, une famille ne peut pas prendre la sauvegarde de toute
// la base). Aucun secret : ni mot de passe, ni code PIN, ni clé, ni session.
// Les notes privées des autres membres n'y sont pas, ni leur budget perso,
// leurs abonnements et projets privés.

// sectionExport : une partie du fichier (« courses_listes »…).
type sectionExport struct {
	nom  string
	lire func(ctx context.Context, q *donnees.Queries, famille, membre int64) (any, error)
}

// s : raccourci pour une requête « par famille ».
func s[T any](nom string, f func(*donnees.Queries, context.Context, int64) ([]T, error)) sectionExport {
	return sectionExport{nom: nom, lire: func(ctx context.Context, q *donnees.Queries, famille, _ int64) (any, error) {
		return f(q, ctx, famille)
	}}
}

// sm : une requête « par famille et membre » (ce qui est privé à un autre
// membre n'est pas exporté : budget perso, abonnements et projets privés).
func sm[T, P any](nom string, f func(*donnees.Queries, context.Context, P) ([]T, error), params func(famille, membre int64) P) sectionExport {
	return sectionExport{nom: nom, lire: func(ctx context.Context, q *donnees.Queries, famille, membre int64) (any, error) {
		return f(q, ctx, params(famille, membre))
	}}
}

var sectionsExport = []sectionExport{
	s("membres", (*donnees.Queries).ExportMembres),
	s("courses_listes", (*donnees.Queries).ExportListes),
	s("courses_produits", (*donnees.Queries).ExportProduits),
	s("courses_produits_connus", (*donnees.Queries).ExportProduitsConnus),
	s("taches_categories", (*donnees.Queries).ExportCategoriesTaches),
	s("taches", (*donnees.Queries).ExportTaches),
	s("taches_faites", (*donnees.Queries).ExportTachesFaites),
	s("agenda_evenements", (*donnees.Queries).ExportEvenements),
	s("agenda_participants", (*donnees.Queries).ExportEvenementsMembres),
	s("agenda_contacts", (*donnees.Queries).ExportEvenementsContacts),
	s("menus_repas", (*donnees.Queries).ExportRepas),
	s("menus_recettes", (*donnees.Queries).ExportRecettes),
	sm("budget_operations", (*donnees.Queries).ExportOperations, func(f, m int64) donnees.ExportOperationsParams {
		return donnees.ExportOperationsParams{FamilleID: f, Membre: m}
	}),
	sm("budget_recurrences", (*donnees.Queries).ExportRecurrences, func(f, m int64) donnees.ExportRecurrencesParams {
		return donnees.ExportRecurrencesParams{FamilleID: f, Membre: m}
	}),
	sm("budget_plafonds", (*donnees.Queries).ExportBudgetPlafonds, func(f, m int64) donnees.ExportBudgetPlafondsParams {
		return donnees.ExportBudgetPlafondsParams{FamilleID: f, Membre: m}
	}),
	sm("abonnements", (*donnees.Queries).ExportAbonnements, func(f, m int64) donnees.ExportAbonnementsParams {
		return donnees.ExportAbonnementsParams{FamilleID: f, Membre: m}
	}),
	s("enfants_missions", (*donnees.Queries).ExportMissions),
	s("enfants_missions_faites", (*donnees.Queries).ExportMissionsFaites),
	s("enfants_recompenses", (*donnees.Queries).ExportRecompenses),
	s("enfants_echanges", (*donnees.Queries).ExportEchanges),
	s("enfants_etoiles_bonus", (*donnees.Queries).ExportEtoilesBonus),
	s("notes_categories", (*donnees.Queries).ExportCategoriesNotes),
	{nom: "notes", lire: func(ctx context.Context, q *donnees.Queries, famille, membre int64) (any, error) {
		return q.ExportNotes(ctx, donnees.ExportNotesParams{FamilleID: famille, AuteurID: sql.NullInt64{Int64: membre, Valid: true}})
	}},
	s("contacts", (*donnees.Queries).ExportContacts),
	s("cartes_fidelite", (*donnees.Queries).ExportCartes),
	s("vehicules", (*donnees.Queries).ExportVehicules),
	s("animaux", (*donnees.Queries).ExportAnimaux),
	s("echeances", (*donnees.Queries).ExportEcheances),
	sm("carnet", (*donnees.Queries).ExportCarnet, func(f, m int64) donnees.ExportCarnetParams {
		return donnees.ExportCarnetParams{FamilleID: f, Membre: m}
	}),
	sm("projets", (*donnees.Queries).ExportProjets, func(f, m int64) donnees.ExportProjetsParams {
		return donnees.ExportProjetsParams{FamilleID: f, Membre: m}
	}),
	sm("projets_etapes", (*donnees.Queries).ExportProjetEtapes, func(f, m int64) donnees.ExportProjetEtapesParams {
		return donnees.ExportProjetEtapesParams{FamilleID: f, Membre: m}
	}),
}

// exporter : le fichier, écrit section par section (pas tout en mémoire).
func (a *App) exporter(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	nomFichier := "familycocon-" + time.Now().In(c.Lieu).Format("2006-01-02") + ".json"
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="`+nomFichier+`"`)
	entete, _ := json.Marshal(map[string]any{
		"format": "familycocon-export", "version": 1, "logiciel": version.Version,
		"exporte_le": time.Now().UTC().Format(time.RFC3339),
		"famille":    map[string]string{"nom": c.Famille.Nom, "fuseau": c.Famille.Fuseau, "devise": c.Famille.Devise},
	})
	// L'en-tête sans son « } » final : les sections suivent.
	_, _ = w.Write(entete[:len(entete)-1])
	for _, sec := range sectionsExport {
		lignes, err := sec.lire(ctx, a.Q, c.Famille.ID, c.Membre.ID)
		if err != nil {
			// Réponse déjà commencée : on ne peut plus changer le statut.
			a.Journal.Error("export : section illisible", "section", sec.nom, "erreur", err)
			_, _ = w.Write([]byte(`,"erreur":"export incomplet"}`))
			return
		}
		b, err := json.Marshal(lignes)
		if err != nil {
			a.Journal.Error("export", "section", sec.nom, "erreur", err)
			return
		}
		_, _ = w.Write([]byte("," + strconv.Quote(sec.nom) + ":"))
		_, _ = w.Write(b)
	}
	_, _ = w.Write([]byte("}\n"))
	a.Journal.Info("export de la famille", "famille", c.Famille.ID, "par", c.Membre.ID)
}
