-- CF-004, CF-006, CF-007, CF-008 : organisations, utilisateurs, sessions, invitations.
CREATE TABLE organizations (
  id            UUID PRIMARY KEY,
  name          TEXT        NOT NULL CHECK (length(name) BETWEEN 2 AND 160),
  slug          TEXT        NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) BETWEEN 2 AND 63),
  legal_name    TEXT,
  phone         TEXT,
  email         CITEXT,
  address       TEXT,
  city          TEXT,
  country_code  CHAR(2)     NOT NULL DEFAULT 'SN',
  currency      CHAR(3)     NOT NULL DEFAULT 'XOF',
  timezone      TEXT        NOT NULL DEFAULT 'Africa/Dakar',
  locale        TEXT        NOT NULL DEFAULT 'fr-SN',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  row_version   INT         NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX organizations_slug_uq ON organizations (slug);
CREATE UNIQUE INDEX organizations_name_uq ON organizations (lower(name));
CREATE TRIGGER trg_org_updated BEFORE UPDATE ON organizations
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE organization_settings (
  organization_id          UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
  budget_alert_thresholds  SMALLINT[]  NOT NULL DEFAULT '{80,90,100}',
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_org_settings_updated BEFORE UPDATE ON organization_settings
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Un utilisateur appartient à UNE seule organisation : le rôle est porté par l'utilisateur.
CREATE TABLE users (
  id                 UUID PRIMARY KEY,
  organization_id    UUID        NOT NULL REFERENCES organizations(id),
  email              CITEXT,
  phone              TEXT CHECK (phone ~ '^\+[1-9][0-9]{6,14}$'),
  full_name          TEXT        NOT NULL CHECK (length(full_name) BETWEEN 2 AND 160),
  role               TEXT        NOT NULL
                     CHECK (role IN ('owner','manager','accountant','site_worker','viewer')),
  password_hash      TEXT,                         -- argon2id ; NULL tant que l'invitation n'est pas acceptée
  status             TEXT        NOT NULL DEFAULT 'active'
                     CHECK (status IN ('invited','active','suspended')),
  failed_login_count SMALLINT    NOT NULL DEFAULT 0,
  locked_until       TIMESTAMPTZ,
  last_login_at      TIMESTAMPTZ,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  suspended_at       TIMESTAMPTZ,
  CONSTRAINT users_contact_chk CHECK (email IS NOT NULL OR phone IS NOT NULL),
  CONSTRAINT users_password_chk CHECK (status = 'invited' OR password_hash IS NOT NULL),
  CONSTRAINT users_org_id_uq UNIQUE (organization_id, id)
);
-- Identifiants de connexion uniques sur toute la plateforme.
CREATE UNIQUE INDEX users_email_uq ON users (email) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX users_phone_uq ON users (phone) WHERE phone IS NOT NULL;
CREATE INDEX users_org_idx ON users (organization_id, status);
CREATE TRIGGER trg_users_updated BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sessions (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL,
  user_id         UUID        NOT NULL,
  token_hash      BYTEA       NOT NULL UNIQUE,     -- SHA-256 du jeton opaque
  remember        BOOLEAN     NOT NULL DEFAULT false,
  user_agent      TEXT,
  ip              TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at      TIMESTAMPTZ NOT NULL,
  revoked_at      TIMESTAMPTZ,
  FOREIGN KEY (organization_id, user_id) REFERENCES users (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX sessions_user_active_idx ON sessions (organization_id, user_id) WHERE revoked_at IS NULL;

CREATE TABLE invitations (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL REFERENCES organizations(id),
  user_id         UUID        NOT NULL,            -- utilisateur pré-créé en statut 'invited'
  token_hash      BYTEA       NOT NULL UNIQUE,
  invited_by      UUID        NOT NULL,
  expires_at      TIMESTAMPTZ NOT NULL,            -- 7 jours, usage unique
  accepted_at     TIMESTAMPTZ,
  revoked_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, user_id)    REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, invited_by) REFERENCES users (organization_id, id)
);
CREATE INDEX invitations_org_pending_idx ON invitations (organization_id)
  WHERE accepted_at IS NULL AND revoked_at IS NULL;
