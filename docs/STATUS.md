# Avancement par ticket — Phase 1

État vérifié le 7 novembre 2026. « Tests » renvoie aux tests automatisés qui prouvent les critères d'acceptation.

Aucun ticket n'est marqué « Done » au sens de la Definition of Done du backlog : **le code n'est ni revu ni fusionné**, et la CI n'a encore jamais tourné sur GitHub (seules ses commandes ont été exécutées en local, sauf le job `screens` dans sa forme CI).

| Ticket | Résultat | Tests | Réserve |
|---|---|---|---|
| CF-001 Socle Go | Fait | `TestCF001_Healthz`, config, image Docker construite | Structure par fichiers dans chaque module (voir DECISIONS) |
| CF-002 Migrations, sqlc | Fait | `TestCF002_*` (up/down/up, RLS, `organization_id`, FK composites) | |
| CF-003 Isolation multi-tenant | Fait | `TestCF003_*` : 26 routes attaquées depuis un autre tenant, RLS sans contexte | |
| CF-004 Authentification | Fait | `TestCF004_*`, `TestCF004_SlidingSessionRenewsCookie` ; mot de passe oublié (hors backlog) : `TestPasswordReset*`, `password-reset.spec.ts` | L'inscription révèle un identifiant déjà pris ; mot de passe oublié par email seulement, rien pour un compte sans email (voir DECISIONS) |
| CF-005 RBAC, audit | Fait | Table de vérité `rbac_test.go`, `TestCF005_*` | |
| CF-006 Organisation | Fait | `TestCF006_Organization`, écran Paramètres | |
| CF-007 Collaborateurs | Fait | `TestCF007_Invitations` (usage unique en concurrence, expiration, suspension) | Envoi par email ; SMS non branché, le lien reste à transmettre |
| CF-008 Rôles | Fait | `TestCF008_Roles`, écran Équipe | Rôle `viewer` ajouté pour les maquettes |
| CF-009 Chantiers | Fait | `TestCF009_Projects` (filtres, recherche, pagination, verrou optimiste) | |
| CF-010 Membres | Fait | `TestCF010_ProjectMembers`, onglet Équipe | |
| CF-011 Phases, statuts | Fait | `TestCF011_*`, trigger SQL de la machine à états | |
| CF-012 Budget | Fait | `TestCF012_BudgetDraft`, éditeur de budget | |
| CF-013 Publication | Fait | `TestCF013_*` (immutabilité en SQL direct, publication concurrente) | À confirmer : manager autorisé à publier |
| CF-014 Révisions | Fait | `TestCF014_Revisions`, comparaison ligne à ligne | |
| CF-015 Dépense | Fait | `TestCF015_*`, formulaire mobile | À confirmer : comptable autorisé à saisir |
| CF-016 Soumission | Fait | `TestCF016_*` (idempotence, concurrence, atomicité) | |
| CF-017 Décision | Fait | `TestCF017_*` (auto-approbation refusée aussi en base, course entre approbateurs), file de validation | |
| CF-018 Paiements | Fait | `TestCF018_*` : 20 paiements concurrents, cumul exact | |
| CF-019 Indicateurs globaux | Fait | `TestCF019_GlobalDashboard` | |
| CF-020 Santé par chantier | Fait | `TestCF020_ProjectHealth` | Reste = budget − approuvé, sans les engagements (hors backlog) |
| CF-021 Catalogue | Fait | `TestCF021_Materials` | |
| CF-022 Mouvements | Fait | `TestCF022_*` : sorties concurrentes, atomicité, absence d'interblocage | Pas de saisie de mouvement hors ligne |
| CF-023 Soldes, historique | Fait | `TestCF023_*`, rapprochement solde / journal | |
| CF-024 Tâches | Fait | `TestCF024_Tasks` | Vue liste avec filtres ; pas de board à colonnes |
| CF-025 Avancement | Fait | `TestCF025_TaskProgress`, `TestResolveProgress` | |
| CF-026 Rapports | Fait | `TestCF026_DailyReports` ; vérifié dans le navigateur, serveurs arrêtés | |
| CF-027 Pièces jointes | Fait | `TestCF027_*`, `TestS3PresignedRoundTrip` sur S3 réel, signature des octets vérifiée | Pas d'antivirus |
| CF-028 Exports | Fait | `TestCF028_*` : totaux, filtres, injection de formule, refus au-delà de 50 000 lignes | Exports synchrones ; contenu du PDF relu par extraction de texte (montants égaux à l'API, accents corrects), mise en page non contrôlée à l'œil |
| CF-029 PWA | Fait | Playwright sur 320, 390, 768 et 1440 px ; manifest et service worker | Installation non testée sur un vrai téléphone iOS ou Android |
| CF-030 Hors connexion | Fait | 32 tests de la file (`sync.test.ts`) ; vérifié dans le navigateur, API et PWA réellement arrêtées | Conflit couvert par les tests de la file et de l'API, pas parcouru dans le navigateur ; `navigator.onLine` simulé pour le bandeau |
| CF-031 Observabilité, sauvegardes, CI | **Partiel** | Logs, métriques et traces vérifiés ; `make backup-test` restaure réellement | **Pas de pgBackRest ni de déploiement : l'hébergeur n'est pas choisi.** Le RPO de 5 minutes n'est donc pas tenu. Pas de Sentry, pas de pipeline CD |
| CF-032 Tests de bout en bout | Fait | `TestCF032_CriticalJourney`, concurrence, RBAC, k6 : P95 de 142 ms à 212 req/s sur 50 000 dépenses | Charge mesurée sur un portable, à refaire sur l'infrastructure cible |

## Phase 2

| Epic | Résultat | Tests | Réserve |
|---|---|---|---|
| E1 Demandes de prix | Fait | `TestE1_*` : cycle de vie, liens hachés et renouvelés, versions d'offre immuables, date limite, droits, isolation, numérotation concurrente ; `rfq.spec.ts` sur 4 formats | Sélection et commande : voir E3 |
| E2 Gestion des fournisseurs | Fait | `TestE2_*` : fiche normalisée, archivage, règles de doublon (dont concurrence), contacts, conditions historisées et immuables, historique filtré par chantier, droits, isolation ; `suppliers.spec.ts` sur 4 formats | |
| E3 Bons de commande | Fait | `TestE3_*` : scores et classement, sélection (consultation close, dernière version), génération, taxes et totaux, workflow et historique, pas d'auto-approbation, prix masqués au chef de chantier, réceptions partielles, idempotence, bon de livraison unique, réceptions concurrentes, entrée en stock, cohérence commandé / reçu / restant, isolation ; `purchase-orders.spec.ts` sur 4 formats | Pas de facture fournisseur ni de PDF (voir DECISIONS) |

## Écrans

Les 15 écrans suivent les maquettes de `docs/ChantierFlow_HTML_Figma`. Écarts et fonctions affichées « Bientôt » : voir `DECISIONS.md`, section « Écrans alignés sur les maquettes ». Vérification : `npm run test:e2e`, sur 4 formats, parcours en ligne et hors connexion.
