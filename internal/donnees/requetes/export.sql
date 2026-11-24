-- Export complet d'une famille (Ma famille > Exporter) : toutes ses donnees,
-- sans aucun secret (ni mot de passe, ni code PIN, ni cle, ni session).

-- name: ExportMembres :many
SELECT id, nom, role, couleur, naissance, cree_le FROM membres WHERE famille_id = ? AND actif = 1 ORDER BY id;

-- name: ExportListes :many
SELECT * FROM listes WHERE famille_id = ?;

-- name: ExportProduitsConnus :many
SELECT * FROM produits_connus WHERE famille_id = ?;

-- name: ExportTaches :many
SELECT * FROM taches WHERE famille_id = ?;

-- name: ExportCategoriesTaches :many
SELECT * FROM categories_taches WHERE famille_id = ?;

-- name: ExportEvenements :many
SELECT * FROM evenements WHERE famille_id = ?;

-- name: ExportRepas :many
SELECT * FROM repas WHERE famille_id = ?;

-- name: ExportRecettes :many
SELECT * FROM recettes WHERE famille_id = ?;

-- Budget, abonnements, projets : le commun et ce qui est a ce membre
-- (le budget perso des autres n'y est pas).
-- name: ExportOperations :many
SELECT * FROM operations WHERE famille_id = ? AND perso_de IN (0, sqlc.arg(membre));

-- name: ExportRecurrences :many
SELECT * FROM recurrences WHERE famille_id = ? AND perso_de IN (0, sqlc.arg(membre));

-- name: ExportAbonnements :many
SELECT a.* FROM abonnements a JOIN recurrences r ON r.id = a.recurrence_id
WHERE a.famille_id = ? AND r.perso_de IN (0, sqlc.arg(membre));

-- name: ExportBudgetPlafonds :many
SELECT * FROM budget_plafonds WHERE famille_id = ? AND perso_de IN (0, sqlc.arg(membre));

-- name: ExportMissions :many
SELECT * FROM missions WHERE famille_id = ?;

-- name: ExportRecompenses :many
SELECT * FROM recompenses WHERE famille_id = ?;

-- name: ExportEtoilesBonus :many
SELECT * FROM etoiles_bonus WHERE famille_id = ?;

-- name: ExportContacts :many
SELECT * FROM contacts WHERE famille_id = ?;

-- name: ExportCartes :many
SELECT * FROM cartes WHERE famille_id = ?;

-- name: ExportEcheances :many
SELECT * FROM echeances WHERE famille_id = ?;

-- name: ExportCarnet :many
SELECT * FROM carnet c WHERE c.famille_id = sqlc.arg(famille_id) AND NOT (c.module = 'projets' AND c.objet_id IN
  (SELECT p.id FROM projets p WHERE p.famille_id = sqlc.arg(famille_id) AND p.perso_de NOT IN (0, sqlc.arg(membre))));

-- name: ExportVehicules :many
SELECT * FROM vehicules WHERE famille_id = ?;

-- name: ExportAnimaux :many
SELECT * FROM animaux WHERE famille_id = ?;

-- name: ExportProjets :many
SELECT * FROM projets WHERE famille_id = ? AND perso_de IN (0, sqlc.arg(membre));

-- name: ExportProjetEtapes :many
SELECT e.* FROM projet_etapes e JOIN projets p ON p.id = e.projet_id
WHERE e.famille_id = ? AND p.perso_de IN (0, sqlc.arg(membre));

-- name: ExportProduits :many
SELECT produits.* FROM produits JOIN listes ON listes.id = produits.liste_id WHERE listes.famille_id = ?;

-- name: ExportTachesFaites :many
SELECT taches_faites.* FROM taches_faites JOIN taches ON taches.id = taches_faites.tache_id WHERE taches.famille_id = ?;

-- name: ExportEvenementsMembres :many
SELECT evenements_membres.* FROM evenements_membres JOIN evenements ON evenements.id = evenements_membres.evenement_id
WHERE evenements.famille_id = ?;

-- name: ExportEvenementsContacts :many
SELECT evenements_contacts.* FROM evenements_contacts JOIN evenements ON evenements.id = evenements_contacts.evenement_id
WHERE evenements.famille_id = ?;

-- name: ExportMissionsFaites :many
SELECT missions_faites.* FROM missions_faites JOIN missions ON missions.id = missions_faites.mission_id WHERE missions.famille_id = ?;

-- name: ExportEchanges :many
SELECT echanges.* FROM echanges JOIN recompenses ON recompenses.id = echanges.recompense_id WHERE recompenses.famille_id = ?;

-- Notes : les notes partagees et les notes privees de celui qui exporte.
-- name: ExportCategoriesNotes :many
SELECT * FROM categories_notes WHERE famille_id = ?;

-- name: ExportNotes :many
SELECT * FROM notes WHERE famille_id = ? AND (privee = 0 OR auteur_id = ?);
