#!/usr/bin/env bash
# Prouve qu'une sauvegarde est restaurable : crée un PostgreSQL éphémère, y joue les migrations,
# écrit des données, sauvegarde, restaure dans une base vierge, puis compare. Rien d'autre n'est touché.
set -euo pipefail

cd "$(dirname "$0")/../.."
export BACKUP_PASSPHRASE="${BACKUP_PASSPHRASE:-restore-test-passphrase}"
container="cf-restore-test-$$"
work="$(mktemp -d)"
cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

docker run -d --name "$container" -e POSTGRES_USER=cf_owner -e POSTGRES_PASSWORD=cf_owner \
  -e POSTGRES_DB=source -p 127.0.0.1::5432 postgres:16-alpine >/dev/null
until docker exec "$container" pg_isready -U cf_owner -d source >/dev/null 2>&1; do sleep 1; done
sleep 1
until docker exec "$container" psql -U cf_owner -d source -c 'SELECT 1' >/dev/null 2>&1; do sleep 1; done
port="$(docker port "$container" 5432/tcp | cut -d: -f2)"
docker exec "$container" psql -q -U cf_owner -d source -c "CREATE ROLE cf_app LOGIN PASSWORD 'cf_app'"

MIGRATE_DATABASE_URL="postgres://cf_owner:cf_owner@127.0.0.1:${port}/source?sslmode=disable" \
DATABASE_URL="postgres://cf_app:cf_app@127.0.0.1:${port}/source?sslmode=disable" \
  go run ./cmd/kodi migrate up

docker exec -i "$container" psql -q -v ON_ERROR_STOP=1 -U cf_owner -d source <<'SQL'
INSERT INTO organizations (id, name, slug) VALUES ('11111111-1111-1111-1111-111111111111', 'Diop Bâtiment', 'diop-batiment');
INSERT INTO organization_settings (organization_id) VALUES ('11111111-1111-1111-1111-111111111111');
INSERT INTO users (id, organization_id, email, full_name, role, password_hash)
  VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'moussa@example.test', 'Moussa Diop', 'owner', 'hash');
INSERT INTO audit_logs (id, organization_id, action, entity_type)
  VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', 'restore.test', 'test');
SQL

# Les outils du conteneur évitent toute dépendance à la version de PostgreSQL de l'hôte.
docker cp deploy/backup/backup.sh "$container":/tmp/backup.sh
docker cp deploy/backup/restore.sh "$container":/tmp/restore.sh
docker exec -e BACKUP_PASSPHRASE -e DATABASE_URL="postgres://cf_owner:cf_owner@localhost:5432/source" \
  "$container" bash -c 'command -v openssl >/dev/null || apk add --no-cache openssl >/dev/null; bash /tmp/backup.sh /tmp/backup.dump.enc' 2>/dev/null ||
docker exec -e BACKUP_PASSPHRASE -e DATABASE_URL="postgres://cf_owner:cf_owner@localhost:5432/source" \
  "$container" sh -c 'apk add --no-cache bash openssl >/dev/null && bash /tmp/backup.sh /tmp/backup.dump.enc'

# Le fichier sauvegardé ne doit rien contenir en clair.
if docker exec "$container" grep -q "moussa@example.test" /tmp/backup.dump.enc; then
  echo "ÉCHEC : la sauvegarde n'est pas chiffrée" >&2
  exit 1
fi

docker exec "$container" psql -q -U cf_owner -d source -c 'CREATE DATABASE restored'
docker exec -e BACKUP_PASSPHRASE -e DATABASE_URL="postgres://cf_owner:cf_owner@localhost:5432/restored" \
  "$container" bash /tmp/restore.sh /tmp/backup.dump.enc

# Une mauvaise phrase secrète doit échouer, pas restaurer n'importe quoi.
docker exec "$container" psql -q -U cf_owner -d source -c 'CREATE DATABASE wrongpass'
if docker exec -e BACKUP_PASSPHRASE=mauvaise -e DATABASE_URL="postgres://cf_owner:cf_owner@localhost:5432/wrongpass" \
  "$container" bash /tmp/restore.sh /tmp/backup.dump.enc >/dev/null 2>&1; then
  echo "ÉCHEC : restauration acceptée avec une mauvaise phrase secrète" >&2
  exit 1
fi

check() { docker exec "$container" psql -U cf_owner -d "$1" -Atc "$2"; }
for query in \
  "SELECT count(*) FROM organizations WHERE slug = 'diop-batiment'" \
  "SELECT email FROM users WHERE full_name = 'Moussa Diop'" \
  "SELECT count(*) FROM audit_logs WHERE action = 'restore.test'" \
  "SELECT count(*) FROM pg_policies WHERE policyname = 'tenant_isolation'" \
  "SELECT count(*) FROM pg_trigger WHERE tgname = 'trg_budget_versions_immutable'" \
  "SELECT max(version) FROM schema_migrations"; do
  a="$(check source "$query")"
  b="$(check restored "$query")"
  if [ "$a" != "$b" ] || [ -z "$b" ]; then
    echo "ÉCHEC : écart sur « $query » : source=$a restaurée=$b" >&2
    exit 1
  fi
  echo "OK  $query = $b"
done
echo "Restauration vérifiée."
