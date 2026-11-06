---
title: "CF-022 — Enregistrer les mouvements de stock"
labels: "phase-1,p1,backend"
---

## Epic
CF-EPIC-07 — Matériaux et stock par chantier

## User story
En tant que chef de chantier, je veux enregistrer entrées, sorties et ajustements.

## Priorité
**P1** — À livrer dans la Phase 1 après les fondations

## Tâches techniques
- [ ] Créer mouvements/items et projection de solde
- [ ] posting transactionnel avec verrouillage
- [ ] interface mouvements
- [ ] tests concurrence.

## Critères d'acceptation
- [ ] Mouvement lié à un chantier
- [ ] mouvement posté immuable
- [ ] sortie ne rend pas stock négatif
- [ ] solde mis à jour atomiquement.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
