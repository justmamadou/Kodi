# Runbook d'exploitation

Cibles : disponibilité 99,5 %, RPO ≤ 5 min, RTO ≤ 2 h (architecture §2.2).

## Mettre en production

Rôles PostgreSQL à créer une fois, avec des secrets du gestionnaire de secrets :

- `cf_owner` : propriétaire du schéma, `BYPASSRLS`, exécute les migrations. **Les fonctions `auth_*` et `purge_expired` lui appartiennent : sans `BYPASSRLS`, plus personne ne peut se connecter.**
- `cf_app` : rôle de l'API, `LOGIN`, jamais `BYPASSRLS` ni propriétaire.

Variables : voir `.env.example`. La configuration est validée au démarrage et le processus s'arrête en listant tout ce qui manque. En production, `COOKIE_SECURE`, le stockage S3 et une `APP_BASE_URL` en HTTPS sont obligatoires. Derrière Caddy : `TRUST_PROXY_HEADERS=true`. `METRICS_ADDR` doit rester sur un réseau privé.

Pour chaque version :

1. Sauvegarder (voir ci-dessous).
2. Sur staging d'abord : `kodi migrate up`, puis les tests de fumée.
3. La première fois, ou si le domaine change : `kodi storage-setup` (bucket et règle CORS limitée à `APP_BASE_URL`).
4. Déployer l'API et la PWA ; `/readyz` doit répondre 200.

**Migrations rétrocompatibles** (expand → migrate → contract) : la version précédente de l'API doit continuer à fonctionner sur le nouveau schéma.

**Retour arrière** : redéployer l'image précédente. Annuler une migration est un dernier recours, car `migrate down` supprime des données : préférez une nouvelle migration corrective.

## Sauvegardes

- **Dump chiffré** : `deploy/backup/backup.sh`. Portable, à copier hors site. La phrase secrète vit dans le gestionnaire de secrets, **jamais à côté de la sauvegarde**.
- **PITR, à mettre en place avec l'hébergeur** : pgBackRest avec archivage WAL. C'est lui qui tient le RPO de 5 minutes. **Non configuré à ce jour : le choix de l'hébergeur est encore ouvert** (décision n° 1 de l'architecture). Jusque-là, le RPO vaut l'intervalle entre deux dumps.
- **Fichiers** : activer le versionnage du bucket et une règle de cycle de vie.

`make backup-test` prouve la restauration de bout en bout sur un PostgreSQL éphémère : la CI l'exécute à chaque changement. **À répéter chaque mois contre une vraie sauvegarde de production.**

## Restaurer

1. Créer une base **vide** ; `restore.sh` refuse d'écraser une base contenant des tables.
2. `DATABASE_URL=... BACKUP_PASSPHRASE=... deploy/backup/restore.sh <fichier>`
3. Vérifier les droits de `cf_app` : le dump ne porte pas les privilèges. Rejouer les `GRANT` et les `REVOKE` de `db/migrations/000010_rls.up.sql` et de `000011_maintenance.up.sql`.
4. Vérifier les propriétaires : les fonctions `auth_*` et `purge_expired` doivent appartenir à un rôle `BYPASSRLS`.
5. Tester une connexion, puis pointer l'API sur la base restaurée.

## Incidents

### Personne ne peut se connecter

- `/readyz` répond 503 : la base est injoignable. Vérifier PostgreSQL, puis les connexions (`SELECT count(*) FROM pg_stat_activity`).
- `/readyz` répond 200 : vérifier que le propriétaire des fonctions `auth_*` a `BYPASSRLS`. Sinon la fonction voit 0 ligne, et tout le monde reçoit « Identifiants incorrects ».
- Un seul utilisateur : compte verrouillé 15 minutes après 5 échecs, ou compte suspendu. Vérifier `locked_until` et `status`.
- Beaucoup de 429 : derrière un proxy, vérifier `TRUST_PROXY_HEADERS=true`. Sinon, tous les clients partagent l'adresse du proxy.

### Les photos ne partent pas

La file reste « En attente d'envoi » et le navigateur signale une erreur CORS : relancer `kodi storage-setup` en vérifiant `APP_BASE_URL`. Vérifier aussi `NEXT_PUBLIC_S3_ORIGIN` (au build de la PWA), qui alimente la CSP. Rien n'est perdu : les fichiers restent sur l'appareil tant que l'envoi n'a pas abouti.

### Lenteurs

Métrique `kodi_http_request_duration_seconds` par route ; `pg_stat_statements` pour la base ; `EXPLAIN ANALYZE` sur la requête en cause. Les paliers de montée en charge sont décrits au §14 de l'architecture.

### Fuite de données inter-entreprises suspectée

**Priorité absolue.** Figer les preuves, lire `audit_logs`, couper l'accès si nécessaire. Trois barrières indépendantes existent (filtre applicatif, FK composites, RLS) : identifier laquelle a cédé. Vérifier qu'aucune table n'a perdu sa politique ; `TestCF002_EveryTenantTableHasRLS` fait ce contrôle. Vérifier les obligations de notification auprès de la CDP (loi n° 2008-12).

## Observabilité

- **Logs** : JSON sur la sortie standard, avec `request_id`, `org_id`, `user_id`, route, statut et durée. Le `request_id` figure aussi dans chaque réponse d'erreur et dans `audit_logs` : c'est le fil à suivre depuis un retour utilisateur. Les erreurs 500 journalisent leur cause sans l'exposer au client.
- **Métriques** : `METRICS_ADDR` expose `/metrics` au format Prometheus (requêtes et latence par route). Alertes conseillées : taux de 5xx > 1 % sur 5 minutes ; P95 > 700 ms sur 10 minutes ; `/readyz` en échec.
- **Traces** : définir `OTEL_EXPORTER_OTLP_ENDPOINT`. Couvre les requêtes HTTP et SQL, sans le texte des requêtes ni leurs paramètres.
- **Sentry n'est pas intégré** (prévu par l'architecture, sans ticket).

## Maintenance courante

- **Partitions d'audit** : créées jusqu'à fin 2028. Pour en ajouter, en tant que `cf_owner` :

  ```sql
  CREATE TABLE audit_logs_2029_01 PARTITION OF audit_logs FOR VALUES FROM ('2029-01-01') TO ('2029-02-01');
  REVOKE ALL ON audit_logs_2029_01 FROM cf_app;
  ```

  Le `REVOKE` est indispensable si des droits par défaut existent : la RLS ne protège pas une partition lue directement.
- **Purge** : l'API retire toutes les 6 heures les clés d'idempotence de plus de 7 jours et les sessions terminées depuis plus de 30 jours.
- **Fichiers orphelins** : un dépôt jamais confirmé reste dans le bucket. Règle de cycle de vie à prévoir côté stockage.
