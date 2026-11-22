-- Double authentification : application (TOTP) avec codes de secours, et
-- passkeys (WebAuthn), pour les comptes à mot de passe.

-- Secret TOTP ('' : pas de double authentification) et dernier pas accepté
-- (un code déjà utilisé est refusé, même dans sa fenêtre de 30 s).
ALTER TABLE membres ADD COLUMN totp TEXT NOT NULL DEFAULT '';
ALTER TABLE membres ADD COLUMN totp_dernier INTEGER NOT NULL DEFAULT 0;

CREATE TABLE codes_secours (
  membre_id  INTEGER NOT NULL REFERENCES membres(id) ON DELETE CASCADE,
  empreinte  BLOB    NOT NULL,
  cree_le    INTEGER NOT NULL,
  utilise_le INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (membre_id, empreinte)
);

CREATE TABLE passkeys (
  id           INTEGER PRIMARY KEY,
  membre_id    INTEGER NOT NULL REFERENCES membres(id) ON DELETE CASCADE,
  identifiant  BLOB    NOT NULL UNIQUE,
  cle_publique BLOB    NOT NULL,       -- PKIX (DER)
  algorithme   INTEGER NOT NULL,       -- numéro COSE
  compteur     INTEGER NOT NULL DEFAULT 0,
  domaine      TEXT    NOT NULL,
  nom          TEXT    NOT NULL,
  cree_le      INTEGER NOT NULL,
  utilise_le   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX passkeys_membre ON passkeys(membre_id);
