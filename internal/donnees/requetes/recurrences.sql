-- Operations qui reviennent, et abonnements.

-- Toutes (ecriture des occurrences).
-- name: RecurrencesFamille :many
SELECT * FROM recurrences WHERE famille_id = ? ORDER BY sens DESC, montant DESC;

-- name: RecurrenceParID :one
SELECT * FROM recurrences WHERE id = ? AND famille_id = ?;

-- name: CompterRecurrences :one
SELECT count(*) FROM recurrences WHERE famille_id = ?;

-- name: CreerRecurrence :one
INSERT INTO recurrences (famille_id, perso_de, virement, sens, montant, categorie, libelle, membre_id, frequence, debut, fin, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- Seulement si " faites " n'a pas bouge depuis la lecture (une occurrence
-- ecrite entre-temps ne doit pas etre ecrite une seconde fois).
-- name: ModifierRecurrence :execrows
UPDATE recurrences SET sens = sqlc.arg(sens), montant = sqlc.arg(montant), categorie = sqlc.arg(categorie),
  libelle = sqlc.arg(libelle), membre_id = sqlc.arg(membre_id), frequence = sqlc.arg(frequence),
  debut = sqlc.arg(debut), fin = sqlc.arg(fin), faites = sqlc.arg(faites),
  nouveau_montant = sqlc.arg(nouveau_montant), nouveau_le = sqlc.arg(nouveau_le)
WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id) AND faites = sqlc.arg(faites_avant);

-- name: ArreterRecurrence :exec
UPDATE recurrences SET fin = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerRecurrence :exec
DELETE FROM recurrences WHERE id = ? AND famille_id = ?;

-- La date du nouveau montant est arrivee : il devient le montant.
-- name: AppliquerNouveauMontant :exec
UPDATE recurrences SET montant = nouveau_montant, nouveau_montant = 0, nouveau_le = ''
WHERE id = ? AND famille_id = ? AND nouveau_le <> '';

-- name: AvancerRecurrence :execrows
UPDATE recurrences SET faites = faites + 1 WHERE id = ? AND famille_id = ? AND faites = ?;

-- name: CreerOccurrence :one
INSERT INTO operations (famille_id, perso_de, sens, montant, categorie, libelle, jour, membre_id, recurrence_id, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- Abonnements vus par un membre (les prives des autres non) ; membre = -1 :
-- tous (rappels, envoyes a chacun selon ce qu'il voit).
-- name: ListeAbonnements :many
SELECT a.id, a.emoji, a.engagement_fin, a.site, a.notes, r.id AS recurrence_id, r.montant, r.categorie, r.libelle,
  r.membre_id, r.frequence, r.debut, r.fin, r.faites, r.perso_de, r.nouveau_montant, r.nouveau_le
FROM abonnements a JOIN recurrences r ON r.id = a.recurrence_id
WHERE a.famille_id = sqlc.arg(famille_id) AND (sqlc.arg(membre) = -1 OR r.perso_de IN (0, sqlc.arg(membre)))
ORDER BY r.libelle COLLATE NOCASE;

-- name: AbonnementParID :one
SELECT * FROM abonnements WHERE id = ? AND famille_id = ?;

-- name: AbonnementDeRecurrence :one
SELECT * FROM abonnements WHERE recurrence_id = ? AND famille_id = ?;

-- Les recurrences qui sont des abonnements (une requete pour toute la page).
-- name: RecurrencesAbonnees :many
SELECT recurrence_id FROM abonnements WHERE famille_id = ?;

-- name: CreerAbonnement :one
INSERT INTO abonnements (famille_id, recurrence_id, emoji, engagement_fin, site, notes, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierAbonnement :exec
UPDATE abonnements SET emoji = ?, engagement_fin = ?, site = ?, notes = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerOccurrences :execrows
DELETE FROM operations WHERE recurrence_id = ? AND famille_id = ?;

-- Abonnement : paye depuis le debut (operations ecrites par la recurrence).
-- name: TotalRecurrence :one
SELECT count(*) AS nombre, CAST(coalesce(sum(montant), 0) AS INTEGER) AS total
FROM operations WHERE famille_id = ? AND recurrence_id = ?;

-- name: SupprimerRecurrencesPerso :exec
DELETE FROM recurrences WHERE famille_id = ? AND perso_de = ?;

-- Abonnement passe au budget perso (ou revenu au commun) : la recurrence
-- et ses prelevements deja ecrits changent de budget.
-- name: ChangerCompteRecurrence :exec
UPDATE recurrences SET perso_de = ? WHERE id = ? AND famille_id = ?;

-- name: ChangerCompteOccurrences :exec
UPDATE operations SET perso_de = ? WHERE recurrence_id = ? AND famille_id = ?;
