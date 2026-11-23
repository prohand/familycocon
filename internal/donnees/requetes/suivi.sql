-- Suivi : echeances et carnet (vehicules, animaux, prestataires).

-- name: EcheancesObjet :many
SELECT * FROM echeances WHERE famille_id = ? AND module = ? AND objet_id = ? ORDER BY jour;

-- name: EcheancesPeriode :many
SELECT * FROM echeances WHERE famille_id = ? AND module = ? AND jour >= sqlc.arg(du) AND jour < sqlc.arg(au) ORDER BY jour;

-- name: ProchainesEcheancesModule :many
SELECT * FROM echeances WHERE famille_id = ? AND module = ? ORDER BY jour LIMIT 50;

-- name: EcheanceParID :one
SELECT * FROM echeances WHERE id = ? AND famille_id = ?;

-- name: CreerEcheance :one
INSERT INTO echeances (famille_id, module, objet_id, libelle, jour, repetition_mois, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ReporterEcheance :exec
UPDATE echeances SET jour = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerEcheance :exec
DELETE FROM echeances WHERE id = ? AND famille_id = ?;

-- name: SupprimerSuiviObjet :exec
DELETE FROM echeances WHERE famille_id = ? AND module = ? AND objet_id = ?;

-- name: CarnetObjet :many
SELECT * FROM carnet WHERE famille_id = ? AND module = ? AND objet_id = ? ORDER BY jour DESC, id DESC LIMIT 100;

-- name: AjouterCarnet :one
INSERT INTO carnet (famille_id, module, objet_id, jour, libelle, montant, km, note, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: LigneCarnetParID :one
SELECT * FROM carnet WHERE id = ? AND famille_id = ?;

-- name: SupprimerLigneCarnet :exec
DELETE FROM carnet WHERE id = ? AND famille_id = ?;

-- name: SupprimerCarnetObjet :exec
DELETE FROM carnet WHERE famille_id = ? AND module = ? AND objet_id = ?;

-- name: ReporterEcheanceSiJour :execrows
UPDATE echeances SET jour = sqlc.arg(nouveau) WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id) AND jour = sqlc.arg(ancien);

-- name: SupprimerEcheanceSiJour :execrows
DELETE FROM echeances WHERE id = ? AND famille_id = ? AND jour = ?;
