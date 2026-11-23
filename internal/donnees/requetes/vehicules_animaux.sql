-- Vehicules, animaux, prestataires.

-- name: VehiculesFamille :many
SELECT * FROM vehicules WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: VehiculeParID :one
SELECT * FROM vehicules WHERE id = ? AND famille_id = ?;

-- name: CreerVehicule :one
INSERT INTO vehicules (famille_id, nom, genre, immatriculation, modele, kilometrage, notes, garage_id, assureur_id, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierVehicule :exec
UPDATE vehicules SET nom = ?, genre = ?, immatriculation = ?, modele = ?, kilometrage = ?, notes = ?, garage_id = ?, assureur_id = ?
WHERE id = ? AND famille_id = ?;

-- name: NoterKilometrage :exec
UPDATE vehicules SET kilometrage = ? WHERE id = ? AND famille_id = ? AND kilometrage < ?;

-- name: SupprimerVehicule :exec
DELETE FROM vehicules WHERE id = ? AND famille_id = ?;

-- name: AnimauxFamille :many
SELECT * FROM animaux WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: AnimalParID :one
SELECT * FROM animaux WHERE id = ? AND famille_id = ?;

-- name: CreerAnimal :one
INSERT INTO animaux (famille_id, nom, espece, race, naissance, puce, veterinaire, veterinaire_id, notes, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierAnimal :exec
UPDATE animaux SET nom = ?, espece = ?, race = ?, naissance = ?, puce = ?, veterinaire = ?, veterinaire_id = ?, notes = ?
WHERE id = ? AND famille_id = ?;

-- name: SupprimerAnimal :exec
DELETE FROM animaux WHERE id = ? AND famille_id = ?;

-- Noms seuls (rappels : pas une requete par echeance).
-- name: NomsVehicules :many
SELECT id, nom FROM vehicules WHERE famille_id = ?;

-- name: NomsAnimaux :many
SELECT id, nom FROM animaux WHERE famille_id = ?;

-- name: CompterVehicules :one
SELECT count(*) FROM vehicules WHERE famille_id = ?;

-- name: CompterAnimaux :one
SELECT count(*) FROM animaux WHERE famille_id = ?;
