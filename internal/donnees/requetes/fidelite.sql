-- Cartes de fidelite (les plus utilisees en premier).

-- name: CartesFamille :many
SELECT * FROM cartes WHERE famille_id = ? ORDER BY utilisee_le DESC, enseigne COLLATE NOCASE;

-- name: CarteParID :one
SELECT * FROM cartes WHERE id = ? AND famille_id = ?;

-- name: CreerCarte :one
INSERT INTO cartes (famille_id, enseigne, numero, format, couleur, titulaire, notes, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierCarte :exec
UPDATE cartes SET enseigne = ?, numero = ?, format = ?, couleur = ?, titulaire = ?, notes = ? WHERE id = ? AND famille_id = ?;

-- name: CarteUtilisee :exec
UPDATE cartes SET utilisee_le = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerCarte :exec
DELETE FROM cartes WHERE id = ? AND famille_id = ?;

-- name: CompterCartes :one
SELECT count(*) FROM cartes WHERE famille_id = ?;
