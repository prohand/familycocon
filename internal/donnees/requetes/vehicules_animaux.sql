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
-- Les vehicules encore a la famille (rappels : pas ceux qui sont partis).
-- name: NomsVehicules :many
SELECT id, nom FROM vehicules WHERE famille_id = ? AND vendu_le = '';

-- name: ChangerVenduVehicule :exec
UPDATE vehicules SET vendu_le = sqlc.arg(vendu_le) WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id);

-- name: NomsAnimaux :many
SELECT id, nom FROM animaux WHERE famille_id = ?;

-- name: CompterVehicules :one
SELECT count(*) FROM vehicules WHERE famille_id = ?;

-- name: CompterAnimaux :one
SELECT count(*) FROM animaux WHERE famille_id = ?;

-- Garanties : appareils.

-- name: GarantiesFamille :many
SELECT * FROM garanties WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: GarantieParID :one
SELECT * FROM garanties WHERE id = ? AND famille_id = ?;

-- name: CreerGarantie :one
INSERT INTO garanties (famille_id, nom, marque, achat, prix, garantie_mois, magasin, magasin_id, facture, notes, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierGarantie :exec
UPDATE garanties SET nom = ?, marque = ?, achat = ?, prix = ?, garantie_mois = ?, magasin = ?, magasin_id = ?, facture = ?, notes = ?
WHERE id = ? AND famille_id = ?;

-- name: SupprimerGarantie :exec
DELETE FROM garanties WHERE id = ? AND famille_id = ?;

-- name: NomsGaranties :many
SELECT id, nom FROM garanties WHERE famille_id = ?;

-- name: CompterGaranties :one
SELECT count(*) FROM garanties WHERE famille_id = ?;

-- Ou vont les depenses du vehicule dans le budget ('', 'commun', 'perso').
-- name: ChangerBudgetVehicule :exec
UPDATE vehicules SET budget = sqlc.arg(budget), budget_perso_de = sqlc.arg(budget_perso_de)
WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id);
