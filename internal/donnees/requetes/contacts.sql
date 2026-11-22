-- Contacts (adresses, anniversaires).

-- name: ContactsFamille :many
SELECT * FROM contacts WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: ContactsAvecNaissance :many
SELECT * FROM contacts WHERE famille_id = ? AND naissance <> '';

-- name: ContactParID :one
SELECT * FROM contacts WHERE id = ? AND famille_id = ?;

-- name: CreerContact :one
INSERT INTO contacts (famille_id, nom, relation, naissance, telephone, email, adresse, notes, idees, cree_le, modifie_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierContact :exec
UPDATE contacts SET nom = ?, relation = ?, naissance = ?, telephone = ?, email = ?, adresse = ?, notes = ?, idees = ?, modifie_le = ?
WHERE id = ? AND famille_id = ?;

-- name: SupprimerContact :exec
DELETE FROM contacts WHERE id = ? AND famille_id = ?;

-- name: CompterContacts :one
SELECT count(*) FROM contacts WHERE famille_id = ?;
