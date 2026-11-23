-- Agenda.

-- name: EvenementsPeriode :many
SELECT * FROM evenements
WHERE famille_id = sqlc.arg(famille_id) AND debut < sqlc.arg(fin_periode)
  AND (fin >= sqlc.arg(debut_periode)
       OR (repetition <> '' AND (repetition_fin = '' OR repetition_fin >= sqlc.arg(jour_min))))
ORDER BY debut;

-- name: CompterEvenements :one
SELECT count(*) FROM evenements WHERE famille_id = ?;

-- name: EvenementParID :one
SELECT * FROM evenements WHERE id = ? AND famille_id = ?;

-- name: CreerEvenement :one
INSERT INTO evenements (famille_id, titre, lieu, note, debut, fin, journee, repetition, repetition_fin, rappel, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierEvenement :exec
UPDATE evenements SET titre = ?, lieu = ?, note = ?, debut = ?, fin = ?, journee = ?, repetition = ?, repetition_fin = ?, rappel = ?
WHERE id = ? AND famille_id = ?;

-- name: SupprimerEvenement :exec
DELETE FROM evenements WHERE id = ? AND famille_id = ?;

-- name: ParticipantsDeFamille :many
SELECT evenements_membres.* FROM evenements_membres
JOIN evenements ON evenements.id = evenements_membres.evenement_id
WHERE evenements.famille_id = ?;

-- Les participants des evenements affiches seulement (ids deja filtres par famille).
-- name: ParticipantsDesEvenements :many
SELECT * FROM evenements_membres WHERE evenement_id IN (sqlc.slice('evenements'));

-- Les contacts cites par les evenements affiches (ids deja filtres par famille).
-- name: ContactsDesEvenements :many
SELECT ec.evenement_id, c.id, c.nom, c.telephone
FROM evenements_contacts ec JOIN contacts c ON c.id = ec.contact_id
WHERE ec.evenement_id IN (sqlc.slice('evenements')) ORDER BY c.nom COLLATE NOCASE;

-- Liste de choix du formulaire : le strict necessaire.
-- name: ContactsChoixAgenda :many
SELECT id, nom, metier FROM contacts WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: AjouterParticipant :exec
INSERT OR IGNORE INTO evenements_membres (evenement_id, membre_id) VALUES (?, ?);

-- name: ViderParticipants :exec
DELETE FROM evenements_membres WHERE evenement_id = ?;
