-- Budget.

-- name: OperationsPeriode :many
SELECT * FROM operations WHERE famille_id = ? AND jour >= sqlc.arg(du) AND jour <= sqlc.arg(au)
ORDER BY jour DESC, id DESC;

-- name: OperationParID :one
SELECT * FROM operations WHERE id = ? AND famille_id = ?;

-- name: CreerOperation :one
INSERT INTO operations (famille_id, sens, montant, categorie, libelle, jour, membre_id, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierOperation :exec
UPDATE operations SET sens = ?, montant = ?, categorie = ?, libelle = ?, jour = ?, membre_id = ?
WHERE id = ? AND famille_id = ?;

-- name: SupprimerOperation :exec
DELETE FROM operations WHERE id = ? AND famille_id = ?;

-- name: TotauxPeriode :one
SELECT
  CAST(coalesce(sum(CASE WHEN sens = 'revenu' THEN montant ELSE 0 END), 0) AS INTEGER) AS revenus,
  CAST(coalesce(sum(CASE WHEN sens = 'depense' THEN montant ELSE 0 END), 0) AS INTEGER) AS depenses
FROM operations WHERE famille_id = ? AND jour >= sqlc.arg(du) AND jour <= sqlc.arg(au);
