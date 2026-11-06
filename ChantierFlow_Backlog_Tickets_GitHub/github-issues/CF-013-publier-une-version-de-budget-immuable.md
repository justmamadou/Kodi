---
title: "CF-013 — Publier une version de budget immuable"
labels: "phase-1,p0,backend"
---

## Epic
CF-EPIC-04 — Budgets versionnés

## User story
En tant que manager autorisé, je veux publier un budget de référence.

## Priorité
**P0** — Bloquant pour le lancement

## Tâches techniques
- [ ] Créer transaction de publication/supersession
- [ ] protéger les lignes publiées par trigger/contrainte
- [ ] enregistrer auteur/date
- [ ] créer confirmation UI.

## Critères d'acceptation
- [ ] Une seule version Published par chantier
- [ ] publication atomique
- [ ] version publiée non modifiable
- [ ] auteur et date conservés.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
