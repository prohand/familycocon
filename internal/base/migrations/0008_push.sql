-- Notifications Web Push : clé du serveur (VAPID), abonnements des
-- appareils, et choix de chaque membre (modules qui le notifient).

-- Réglages internes du serveur (clé VAPID…) : dans la base, donc dans les
-- sauvegardes (une restauration garde les abonnements valides).
CREATE TABLE serveur (
  nom    TEXT PRIMARY KEY,
  valeur BLOB NOT NULL
);

CREATE TABLE abonnements_push (
  id        INTEGER PRIMARY KEY,
  membre_id INTEGER NOT NULL REFERENCES membres(id) ON DELETE CASCADE,
  adresse   TEXT    NOT NULL UNIQUE,
  p256dh    BLOB    NOT NULL,
  auth      BLOB    NOT NULL,
  cree_le   INTEGER NOT NULL
);
CREATE INDEX abonnements_membre ON abonnements_push(membre_id);

-- Modules qui envoient une notification à ce membre (les courses sont
-- bavardes : pas par défaut).
ALTER TABLE membres ADD COLUMN push_modules TEXT NOT NULL DEFAULT 'agenda,taches,menus,enfants';
