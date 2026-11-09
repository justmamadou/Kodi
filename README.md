# Kodi

Gestion de chantiers pour les PME du BTP : budgets versionnés, dépenses approuvées, demandes de prix aux fournisseurs, stock par chantier et suivi terrain, sur une PWA utilisable sans réseau.

- **Backend** : monolithe modulaire Go, PostgreSQL 16 (sqlc, pgx, golang-migrate), API REST documentée dans [api/openapi.yaml](api/openapi.yaml).
- **Frontend** : PWA Next.js dans [web/](web/).
- **Décisions** : [docs/DECISIONS.md](docs/DECISIONS.md), notamment les écarts entre le backlog et le document d'architecture.
- **Exploitation** : [docs/RUNBOOK.md](docs/RUNBOOK.md).
- **Avancement par ticket** : [docs/STATUS.md](docs/STATUS.md).

## Démarrer en local

Prérequis : Go 1.26, Node 24, Docker.

```bash
docker compose up -d postgres s3 mailpit
```

```bash
set -a; . ./.env.example; set +a; go run ./cmd/kodi migrate up
```

```bash
set -a; . ./.env.example; set +a; go run ./cmd/kodi api
```

Dans un second terminal :

```bash
cd web && npm install && npm run build && npm start
```

La PWA répond sur <http://localhost:3000>, l'API sur <http://localhost:8080>, les emails de développement sur <http://localhost:8025>. Le stockage S3 local écoute sur le port 9100 (RustFS).

## Tests

```bash
make lint
```

```bash
TEST_S3_ENDPOINT=http://localhost:9100 make test
```

```bash
make backup-test
```

```bash
cd web && npm run lint && npm run typecheck && npm test
```

Avec l'API et la PWA démarrées, pour les parcours sur 4 tailles d'écran (utilise le Chrome installé) :

```bash
cd web && npm run test:e2e
```

Charge ciblée, avec un compte existant :

```bash
docker run --rm -i --add-host=host.docker.internal:host-gateway -e BASE_URL=http://host.docker.internal:8080 -e LOGIN=... -e PASSWORD=... grafana/k6:1.3.0 run - < deploy/loadtest/k6.js
```

`internal/e2e` teste l'API complète sur un vrai PostgreSQL, avec le rôle restreint `cf_app` : la RLS, les triggers et les contraintes s'appliquent comme en production. Chaque exécution crée sa propre base, puis la supprime.

## Organisation du code

```text
cmd/kodi/     binaire unique : api | migrate | storage-setup
api/openapi.yaml      contrat de l'API, vérifié contre les routes par un test
db/migrations/        schéma (golang-migrate), embarqué dans le binaire
db/queries/           SQL écrit à la main ; sqlc génère internal/platform/db
internal/<module>/    domain.go, service.go (règles et transactions), handler.go (HTTP)
internal/rbac/        matrice rôle × permission, accès aux chantiers
internal/audit/       journal écrit dans la transaction métier
internal/e2e/         tests de bout en bout, par ticket
web/src/lib/offline/  file d'envoi hors connexion
deploy/               Caddy, rôles PostgreSQL, sauvegarde et restauration, test de charge
```

Un module ne dépend jamais d'un autre module métier : `depguard` le vérifie en CI.

## Règles à connaître avant de modifier le code

- **Le tenant vient de la session, jamais de la requête.** Tout accès passe par `TxManager.InTenantTx`, sans quoi la RLS ne laisse rien voir.
- **Une nouvelle table porte `organization_id`, une FK composite et une politique RLS** (ajoutée dans sa migration). Un test échoue sinon.
- **Montants en `BIGINT`, quantités en `NUMERIC(14,3)`.** Aucun flottant, un test le vérifie.
- **Les règles métier vivent dans les services, et les invariants sont aussi imposés en base** : un bug ou un script ne doit pas pouvoir contourner l'immutabilité d'un budget publié ou la machine à états d'une dépense.
- **Après avoir modifié `db/queries`** : `make generate`. **Après avoir ajouté une route** : documentez-la dans `api/openapi.yaml`, sinon `TestOpenAPIMatchesRoutes` échoue.
