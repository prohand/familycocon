-- Operations qui reviennent, et abonnements.

-- name: RecurrencesFamille :many
SELECT * FROM recurrences WHERE famille_id = ? ORDER BY sens DESC, montant DESC;

-- name: RecurrenceParID :one
SELECT * FROM recurrences WHERE id = ? AND famille_id = ?;

-- name: CreerRecurrence :one
INSERT INTO recurrences (famille_id, sens, montant, categorie, libelle, membre_id, frequence, debut, fin, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierRecurrence :exec
UPDATE recurrences SET sens = ?, montant = ?, categorie = ?, libelle = ?, membre_id = ?, frequence = ?, debut = ?, fin = ?, faites = ?
WHERE id = ? AND famille_id = ?;

-- name: ArreterRecurrence :exec
UPDATE recurrences SET fin = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerRecurrence :exec
DELETE FROM recurrences WHERE id = ? AND famille_id = ?;

-- name: AvancerRecurrence :execrows
UPDATE recurrences SET faites = faites + 1 WHERE id = ? AND faites = ?;

-- name: CreerOccurrence :one
INSERT INTO operations (famille_id, sens, montant, categorie, libelle, jour, membre_id, recurrence_id, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ListeAbonnements :many
SELECT a.id, a.emoji, a.engagement_fin, a.site, a.notes, r.id AS recurrence_id, r.montant, r.categorie, r.libelle,
  r.membre_id, r.frequence, r.debut, r.fin, r.faites
FROM abonnements a JOIN recurrences r ON r.id = a.recurrence_id
WHERE a.famille_id = ? ORDER BY r.libelle COLLATE NOCASE;

-- name: AbonnementParID :one
SELECT * FROM abonnements WHERE id = ? AND famille_id = ?;

-- name: AbonnementDeRecurrence :one
SELECT * FROM abonnements WHERE recurrence_id = ? AND famille_id = ?;

-- name: CreerAbonnement :one
INSERT INTO abonnements (famille_id, recurrence_id, emoji, engagement_fin, site, notes, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierAbonnement :exec
UPDATE abonnements SET emoji = ?, engagement_fin = ?, site = ?, notes = ? WHERE id = ? AND famille_id = ?;
