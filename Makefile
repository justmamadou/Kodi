SQLC_VERSION := v1.31.1
LINT_VERSION := v2.15.0

.PHONY: up down migrate run generate generate-check lint test test-unit build backup-test

up: ## Démarre PostgreSQL, le stockage S3 et Mailpit
	docker compose up -d postgres s3 mailpit

down:
	docker compose down

migrate: ## Applique les migrations
	go run ./cmd/kodi migrate up

run: ## Lance l'API
	go run ./cmd/kodi api

generate: ## Régénère le code d'accès aux données
	cd db && go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

generate-check: generate ## Échoue si le code généré n'est pas à jour
	git diff --exit-code -- internal/platform/db

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(LINT_VERSION) run ./...

test-unit:
	go test -race $$(go list ./... | grep -v /internal/e2e)

test: ## Tous les tests ; nécessite PostgreSQL (make up)
	go test -race -count=1 ./...

build:
	CGO_ENABLED=0 go build -trimpath -o bin/kodi ./cmd/kodi

backup-test: ## Sauvegarde, restaure et vérifie sur une base éphémère
	./deploy/backup/restore-test.sh
