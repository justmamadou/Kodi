# ChantierFlow — Tickets GitHub Projects

## Outil gratuit recommandé
**GitHub Issues + GitHub Projects** : gestion du backlog, vues Table/Board/Roadmap, liens directs avec commits et pull requests. Adapté à une équipe qui développe déjà sur GitHub. Les limites des plans évoluent : vérifier GitHub Pricing.

## Contenu
- `EPICS.md` : registre des 10 Epics.
- `backlog.csv` : backlog tabulaire (35 User Stories).
- `github-issues/` : une description Markdown par ticket, avec tâches et critères d'acceptation.
- `create-github-issues.sh` : création des issues avec GitHub CLI.

## Mise en place
1. Créer un repository privé GitHub `chantierflow`.
2. Créer un Project nommé `ChantierFlow — Phase 1`.
3. Installer GitHub CLI et lancer `gh auth login`.
4. Extraire cette archive, puis depuis le dossier :
   `GH_REPO=OWNER/chantierflow ./create-github-issues.sh`
5. Ajouter les issues au Project et configurer les champs `Status`, `Priority`, `Epic`, `Sprint`, `Estimate`.

Le script crée les labels et issues. Il ne crée pas automatiquement le Project ni les champs personnalisés.

## Vues et workflow conseillés
- Table : backlog complet, groupé par Epic.
- Board : Backlog → Ready → In progress → In review → Done.
- Roadmap : regroupement par Epic ou Sprint.
- Priorité : P0 / P1 / P2.
- Sprints indicatifs : S0 fondations ; S1 organisation/chantiers ; S2 budgets ; S3 dépenses ; S4 dashboard ; S5 stock/opérations ; S6 documents/exports ; S7 stabilisation/release.

## Priorisation
P0 : nécessaire au lancement ou bloquant pour un parcours critique.
P1 : valeur Phase 1, réalisable après les fondations.
Les estimations doivent être faites par l'équipe après refinement.
