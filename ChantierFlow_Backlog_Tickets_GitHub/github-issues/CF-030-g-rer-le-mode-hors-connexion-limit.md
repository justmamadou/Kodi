---
title: "CF-030 — Gérer le mode hors connexion limité"
labels: "phase-1,p1,backend"
---

## Epic
CF-EPIC-10 — PWA, qualité et mise en production

## User story
En tant que chef de chantier, je veux conserver les brouillons sans réseau.

## Priorité
**P1** — À livrer dans la Phase 1 après les fondations

## Tâches techniques
- [ ] Mettre en cache ressources statiques
- [ ] stocker brouillons localement
- [ ] créer outbox de synchronisation
- [ ] gérer conflits et états réseau.

## Critères d'acceptation
- [ ] Brouillons rapports/tâches conservés
- [ ] synchronisation avec états succès/attente/échec
- [ ] aucune approbation financière offline.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
