-- Budget.

-- Budget d'un compte : perso_de = 0 (commun) ou le membre (perso).
-- name: OperationsPeriode :many
SELECT * FROM operations WHERE famille_id = sqlc.arg(famille_id) AND perso_de = sqlc.arg(compte)
  AND jour >= sqlc.arg(du) AND jour <= sqlc.arg(au)
ORDER BY jour DESC, id DESC;

-- name: OperationParID :one
SELECT * FROM operations WHERE id = ? AND famille_id = ?;

-- name: CreerOperation :one
INSERT INTO operations (famille_id, perso_de, sens, montant, categorie, libelle, jour, membre_id, virement_id, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- Virement modifie : l'autre moitie suit (montant et date).
-- name: ModifierMoitieVirement :exec
UPDATE operations SET montant = ?, jour = ? WHERE id = ? AND famille_id = ?;

-- Virement : chaque moitie connait l'autre.
-- name: LierVirement :exec
UPDATE operations SET virement_id = ? WHERE id = ? AND famille_id = ?;

-- name: ModifierOperation :exec
UPDATE operations SET sens = ?, montant = ?, categorie = ?, libelle = ?, jour = ?, membre_id = ?
WHERE id = ? AND famille_id = ?;

-- name: SupprimerOperation :exec
DELETE FROM operations WHERE id = ? AND famille_id = ?;

-- name: TotauxPeriode :one
SELECT
  CAST(coalesce(sum(CASE WHEN sens = 'revenu' THEN montant ELSE 0 END), 0) AS INTEGER) AS revenus,
  CAST(coalesce(sum(CASE WHEN sens = 'depense' THEN montant ELSE 0 END), 0) AS INTEGER) AS depenses
FROM operations WHERE famille_id = sqlc.arg(famille_id) AND perso_de = sqlc.arg(compte) AND jour >= sqlc.arg(du) AND jour <= sqlc.arg(au);

-- Meme operation ecrite a l'instant par la meme personne (double envoi).
-- name: OperationIdentiqueRecente :one
SELECT count(*) FROM operations
WHERE famille_id = ? AND perso_de = ? AND sens = ? AND montant = ? AND categorie = ? AND libelle = ? AND jour = ?
  AND cree_par = ? AND recurrence_id IS NULL AND cree_le >= ?;

-- Libelles deja tapes (suggestions du formulaire), les plus recents d'abord.
-- name: LibellesRecents :many
SELECT libelle FROM operations WHERE famille_id = ? AND perso_de = ? AND libelle <> ''
GROUP BY libelle ORDER BY max(jour) DESC LIMIT 30;

-- Recherche dans les libelles, sans majuscules ni accents, depuis une date.
-- name: ChercherOperations :many
SELECT * FROM operations
WHERE famille_id = sqlc.arg(famille_id) AND perso_de = sqlc.arg(compte) AND jour >= sqlc.arg(depuis)
  AND instr(plier(libelle), plier(sqlc.arg(motif))) > 0
ORDER BY jour DESC, id DESC LIMIT 200;

-- Totaux par mois, sens et categorie (vue de l'annee).
-- name: TotauxParMois :many
SELECT CAST(substr(jour, 1, 7) AS TEXT) AS mois, sens, categorie, CAST(sum(montant) AS INTEGER) AS total
FROM operations WHERE famille_id = sqlc.arg(famille_id) AND perso_de = sqlc.arg(compte) AND jour >= sqlc.arg(du) AND jour <= sqlc.arg(au)
GROUP BY 1, 2, 3;

-- name: PlafondsFamille :many
SELECT categorie, montant FROM budget_plafonds WHERE famille_id = ? AND perso_de = ?;

-- name: PoserPlafond :exec
INSERT INTO budget_plafonds (famille_id, perso_de, categorie, montant) VALUES (?, ?, ?, ?)
ON CONFLICT (famille_id, perso_de, categorie) DO UPDATE SET montant = excluded.montant;

-- name: RetirerPlafond :exec
DELETE FROM budget_plafonds WHERE famille_id = ? AND perso_de = ? AND categorie = ?;

-- Membre retire : son budget perso part avec lui (personne d'autre ne le voit).
-- name: SupprimerBudgetPerso :exec
DELETE FROM operations WHERE famille_id = ? AND perso_de = ?;

-- name: SupprimerPlafondsPerso :exec
DELETE FROM budget_plafonds WHERE famille_id = ? AND perso_de = ?;

-- Depense du carnet modifiee : la ligne du budget suit.
-- name: ModifierOperationCarnet :exec
UPDATE operations SET montant = ?, libelle = ?, jour = ? WHERE id = ? AND famille_id = ?;
