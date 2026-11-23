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
