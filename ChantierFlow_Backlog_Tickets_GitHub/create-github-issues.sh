#!/usr/bin/env bash
set -euo pipefail
: "${GH_REPO:?Définir GH_REPO au format OWNER/REPO}"
command -v gh >/dev/null || { echo "Installer GitHub CLI (gh)"; exit 1; }
for file in github-issues/*.md; do
  title=$(sed -n 's/^title: "\(.*\)"/\1/p' "$file" | head -1)
  labels=$(sed -n 's/^labels: "\(.*\)"/\1/p' "$file" | head -1)
  args=()
  IFS=',' read -ra parts <<< "$labels"
  for label in "${parts[@]}"; do
    gh label create "$label" --repo "$GH_REPO" --color "ededed" 2>/dev/null || true
    args+=(--label "$label")
  done
  gh issue create --repo "$GH_REPO" --title "$title" --body-file "$file" "${args[@]}"
done
