// Package migrate exécute les migrations golang-migrate embarquées dans le binaire.
package migrate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pilote pgx5://
	"github.com/golang-migrate/migrate/v4/source/iofs"
	dbschema "github.com/justmamadou/Kodi/db"
)

func open(databaseURL string) (*migrate.Migrate, error) {
	source, err := iofs.New(dbschema.Migrations, "migrations")
	if err != nil {
		return nil, err
	}
	url := databaseURL
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(url, prefix) {
			url = "pgx5://" + strings.TrimPrefix(url, prefix)
		}
	}
	return migrate.NewWithSourceInstance("iofs", source, url)
}

func closeAll(m *migrate.Migrate, err error) error {
	sourceErr, dbErr := m.Close()
	return errors.Join(err, sourceErr, dbErr)
}

// Up applique toutes les migrations en attente.
func Up(databaseURL string) error {
	m, err := open(databaseURL)
	if err != nil {
		return err
	}
	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		err = nil
	}
	return closeAll(m, err)
}

// Down annule toutes les migrations.
func Down(databaseURL string) error {
	m, err := open(databaseURL)
	if err != nil {
		return err
	}
	err = m.Down()
	if errors.Is(err, migrate.ErrNoChange) {
		err = nil
	}
	return closeAll(m, err)
}

// Steps applique (n > 0) ou annule (n < 0) n migrations.
func Steps(databaseURL string, n int) error {
	m, err := open(databaseURL)
	if err != nil {
		return err
	}
	err = m.Steps(n)
	if errors.Is(err, migrate.ErrNoChange) {
		err = nil
	}
	return closeAll(m, err)
}

// Version retourne la version courante du schéma.
func Version(databaseURL string) (string, error) {
	m, err := open(databaseURL)
	if err != nil {
		return "", err
	}
	version, dirty, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return "aucune", closeAll(m, nil)
	}
	if err != nil {
		return "", closeAll(m, err)
	}
	return fmt.Sprintf("%d (dirty=%v)", version, dirty), closeAll(m, nil)
}
