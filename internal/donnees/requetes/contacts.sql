-- Contacts : carnet central (adresses, anniversaires, prestataires).

-- name: ContactsFamille :many
SELECT * FROM contacts WHERE famille_id = ? ORDER BY favori DESC, nom COLLATE NOCASE;

-- name: PrestatairesFamille :many
SELECT * FROM contacts WHERE famille_id = ? AND pro = 1 ORDER BY metier COLLATE NOCASE, nom COLLATE NOCASE;

-- name: ContactsAvecNaissance :many
SELECT * FROM contacts WHERE famille_id = ? AND naissance <> '';

-- name: ContactParID :one
SELECT * FROM contacts WHERE id = ? AND famille_id = ?;

-- name: CreerContact :one
INSERT INTO contacts (famille_id, nom, relation, naissance, telephone, tel_fixe, tel_travail, email, adresse, societe, metier, site,
  favori, pro, groupes, notes, idees, cree_le, modifie_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierContact :exec
UPDATE contacts SET nom = ?, relation = ?, naissance = ?, telephone = ?, tel_fixe = ?, tel_travail = ?, email = ?, adresse = ?,
  societe = ?, metier = ?, site = ?, favori = ?, pro = ?, groupes = ?, notes = ?, idees = ?, modifie_le = ?
WHERE id = ? AND famille_id = ?;

-- name: DefinirFavori :exec
UPDATE contacts SET favori = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerContact :exec
DELETE FROM contacts WHERE id = ? AND famille_id = ?;

-- name: CompterContacts :one
SELECT count(*) FROM contacts WHERE famille_id = ?;

-- name: CompterPrestataires :one
SELECT count(*) FROM contacts WHERE famille_id = ? AND pro = 1;

-- name: CompterAdresses :one
SELECT count(*) FROM contacts WHERE famille_id = ? AND adresse <> '';

-- Listes de choix (veterinaire, garage...) : le strict necessaire.
-- name: ContactsChoix :many
SELECT id, nom, pro, metier FROM contacts WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: NomsPrestataires :many
SELECT id, nom FROM contacts WHERE famille_id = ? AND pro = 1;

-- Doublons a l'import : nom et telephones seulement.
-- name: ClesContacts :many
SELECT nom, telephone, tel_fixe, tel_travail FROM contacts WHERE famille_id = ?;

-- Contacts cites par les evenements de l'agenda.

-- name: ContactsEvenementsFamille :many
SELECT ec.evenement_id, c.id, c.nom, c.telephone
FROM evenements_contacts ec JOIN contacts c ON c.id = ec.contact_id
WHERE c.famille_id = ? ORDER BY c.nom COLLATE NOCASE;

-- name: AjouterContactEvenement :exec
INSERT OR IGNORE INTO evenements_contacts (evenement_id, contact_id) VALUES (?, ?);

-- name: ViderContactsEvenement :exec
DELETE FROM evenements_contacts WHERE evenement_id = ?;
