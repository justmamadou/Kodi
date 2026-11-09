# Parcours multi-écrans (Playwright)

Prérequis : l'API et la PWA tournent (voir le README à la racine), ainsi que Mailpit, où le parcours « mot de passe oublié » lit le lien reçu (`MAILPIT_URL`, par défaut <http://localhost:8025>). Lancer l'API avec `AUTH_RATE_PER_MINUTE=600` : la valeur de `.env.example` (20) est atteinte par les inscriptions et connexions des quatre formats. Chaque exécution crée sa propre entreprise de test : aucune préparation de données n'est nécessaire.

```bash
npm run test:e2e
```

Utilise le Chrome installé sur le poste (`channel: "chrome"`), sans télécharger de navigateur. Pour viser un autre environnement : `BASE_URL=https://staging.example npm run test:e2e`.

Quatre formats sont couverts, de 320 px au bureau. Sur chaque écran, le test vérifie l'absence de débordement horizontal, d'erreur JavaScript et de message d'erreur inattendu, ainsi que la taille des cibles tactiles (sous 1024 px : au bureau, les maquettes ont des contrôles plus compacts). Un parcours demande un lien de réinitialisation, le suit depuis l'email, change le mot de passe et se reconnecte avec le nouveau. Un autre coupe le réseau et vérifie qu'une dépense attend sur l'appareil, puis part seule au retour de la connexion.
