-- Vehicules, animaux, prestataires.

-- name: VehiculesFamille :many
SELECT * FROM vehicules WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: VehiculeParID :one
SELECT * FROM vehicules WHERE id = ? AND famille_id = ?;

-- name: CreerVehicule :one
INSERT INTO vehicules (famille_id, nom, genre, immatriculation, modele, kilometrage, notes, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierVehicule :exec
UPDATE vehicules SET nom = ?, genre = ?, immatriculation = ?, modele = ?, kilometrage = ?, notes = ? WHERE id = ? AND famille_id = ?;

-- name: NoterKilometrage :exec
UPDATE vehicules SET kilometrage = ? WHERE id = ? AND famille_id = ? AND kilometrage < ?;

-- name: SupprimerVehicule :exec
DELETE FROM vehicules WHERE id = ? AND famille_id = ?;

-- name: AnimauxFamille :many
SELECT * FROM animaux WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: AnimalParID :one
SELECT * FROM animaux WHERE id = ? AND famille_id = ?;

-- name: CreerAnimal :one
INSERT INTO animaux (famille_id, nom, espece, race, naissance, puce, veterinaire, notes, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierAnimal :exec
UPDATE animaux SET nom = ?, espece = ?, race = ?, naissance = ?, puce = ?, veterinaire = ?, notes = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerAnimal :exec
DELETE FROM animaux WHERE id = ? AND famille_id = ?;

-- name: PrestatairesFamille :many
SELECT * FROM prestataires WHERE famille_id = ? ORDER BY metier COLLATE NOCASE, nom COLLATE NOCASE;

-- name: PrestataireParID :one
SELECT * FROM prestataires WHERE id = ? AND famille_id = ?;

-- name: CreerPrestataire :one
INSERT INTO prestataires (famille_id, nom, metier, telephone, email, adresse, site, notes, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierPrestataire :exec
UPDATE prestataires SET nom = ?, metier = ?, telephone = ?, email = ?, adresse = ?, site = ?, notes = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerPrestataire :exec
DELETE FROM prestataires WHERE id = ? AND famille_id = ?;
