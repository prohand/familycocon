-- Suivi : echeances et carnet (vehicules, animaux, prestataires).

-- name: EcheancesObjet :many
SELECT * FROM echeances WHERE famille_id = ? AND module = ? AND objet_id = ? ORDER BY jour;

-- name: EcheancesPeriode :many
SELECT * FROM echeances WHERE famille_id = ? AND module = ? AND jour >= sqlc.arg(du) AND jour < sqlc.arg(au) ORDER BY jour;

-- La premiere echeance de chaque objet d'un module (alertes des listes) :
-- avec min(), SQLite prend le libelle de la meme ligne.
-- name: PremieresEcheancesModule :many
SELECT objet_id, libelle, CAST(min(jour) AS TEXT) AS jour FROM echeances
WHERE famille_id = ? AND module = ? GROUP BY objet_id;

-- name: CompterEcheancesObjet :one
SELECT count(*) FROM echeances WHERE famille_id = ? AND module = ? AND objet_id = ?;

-- name: EcheanceParID :one
SELECT * FROM echeances WHERE id = ? AND famille_id = ?;

-- name: CreerEcheance :one
INSERT INTO echeances (famille_id, module, objet_id, libelle, jour, repetition_mois, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: SupprimerEcheance :exec
DELETE FROM echeances WHERE id = ? AND famille_id = ?;

-- name: SupprimerSuiviObjet :exec
DELETE FROM echeances WHERE famille_id = ? AND module = ? AND objet_id = ?;

-- name: CarnetObjet :many
SELECT * FROM carnet WHERE famille_id = ? AND module = ? AND objet_id = ? ORDER BY jour DESC, id DESC LIMIT 100;

-- Totaux sur tout le carnet (la liste, elle, s'arrete a 100 lignes).
-- name: TotauxCarnetObjet :one
SELECT CAST(coalesce(sum(montant), 0) AS INTEGER) AS total,
  CAST(coalesce(sum(CASE WHEN jour >= sqlc.arg(debut_annee) AND jour < sqlc.arg(fin_annee) THEN montant ELSE 0 END), 0) AS INTEGER) AS annee
FROM carnet WHERE famille_id = sqlc.arg(famille_id) AND module = sqlc.arg(module) AND objet_id = sqlc.arg(objet_id);

-- name: CompterCarnetObjet :one
SELECT count(*) FROM carnet WHERE famille_id = ? AND module = ? AND objet_id = ?;

-- Meme ligne ecrite a l'instant par la meme personne (double envoi).
-- name: CarnetIdentiqueRecent :one
SELECT count(*) FROM carnet
WHERE famille_id = ? AND module = ? AND objet_id = ? AND jour = ? AND libelle = ? AND montant = ? AND km = ? AND note = ?
  AND cree_par = ? AND cree_le >= ?;

-- name: AjouterCarnet :one
INSERT INTO carnet (famille_id, module, objet_id, jour, libelle, montant, km, litres, note, operation_id, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

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

-- name: ModifierCarnet :exec
UPDATE carnet SET jour = ?, libelle = ?, montant = ?, km = ?, litres = ?, note = ? WHERE id = ? AND famille_id = ?;

-- name: ModifierEcheance :exec
UPDATE echeances SET libelle = ?, jour = ?, repetition_mois = ? WHERE id = ? AND famille_id = ?;

-- Pleins notes (litres et kilometrage) : consommation du vehicule.
-- name: PleinsObjet :many
SELECT km, litres FROM carnet WHERE famille_id = ? AND module = ? AND objet_id = ? AND litres > 0 AND km > 0
ORDER BY km;

-- Kilometrages notes au carnet (cout au kilometre).
-- name: KmCarnetObjet :one
SELECT CAST(coalesce(min(km), 0) AS INTEGER) AS mini, CAST(coalesce(max(km), 0) AS INTEGER) AS maxi
FROM carnet WHERE famille_id = ? AND module = ? AND objet_id = ? AND km > 0;

-- Echeance tenue a jour par un module (fin de garantie) : remplacee.
-- name: SupprimerEcheanceLibelle :exec
DELETE FROM echeances WHERE famille_id = ? AND module = ? AND objet_id = ? AND libelle = ?;

-- Pieces jointes (photos de tickets, factures).

-- name: AjouterPiece :one
INSERT INTO pieces (famille_id, carnet_id, fichier, taille, cree_par, cree_le) VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: PieceParID :one
SELECT * FROM pieces WHERE id = ? AND famille_id = ?;

-- name: PiecesObjet :many
SELECT pieces.* FROM pieces JOIN carnet ON carnet.id = pieces.carnet_id
WHERE carnet.famille_id = ? AND carnet.module = ? AND carnet.objet_id = ? ORDER BY pieces.id;

-- name: PiecesLigne :many
SELECT * FROM pieces WHERE carnet_id = ? AND famille_id = ?;

-- name: CompterPiecesLigne :one
SELECT count(*) FROM pieces WHERE carnet_id = ?;

-- name: CompterPiecesFamille :one
SELECT count(*) FROM pieces WHERE famille_id = ?;

-- name: SupprimerPiece :exec
DELETE FROM pieces WHERE id = ? AND famille_id = ?;

-- Doublon a l'import (Fuelio) : meme jour, libelle, montant et kilometrage.
-- name: CarnetExiste :one
SELECT count(*) FROM carnet
WHERE famille_id = ? AND module = ? AND objet_id = ? AND jour = ? AND libelle = ? AND montant = ? AND km = ?;
