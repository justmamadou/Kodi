---
title: "CF-003 — Garantir l'isolation multi-tenant"
labels: "phase-1,p0,backend"
---

## Epic
CF-EPIC-01 — Fondations, sécurité et multi-tenancy

## User story
En tant qu'utilisateur, je veux accéder uniquement aux données de mon entreprise.

## Priorité
**P0** — Bloquant pour le lancement

## Tâches techniques
- [ ] Ajouter contexte organisation authentifiée
- [ ] auditer toutes les queries tenant-scoped
- [ ] appliquer vérifications sur les ressources
- [ ] écrire tests négatifs inter-tenant.

## Critères d'acceptation
- [ ] Un utilisateur d'une organisation ne lit ni ne modifie aucune ressource d'une autre
- [ ] les tests couvrent projets, budgets, dépenses et fichiers.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
