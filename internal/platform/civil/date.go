// Package civil fournit une date métier sans heure ni fuseau (expense_date, report_date...).
package civil

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const layout = "2006-01-02"

// Date est sérialisée en JSON au format YYYY-MM-DD et stockée en DATE PostgreSQL.
type Date struct{ time.Time }

// Parse lit une date au format YYYY-MM-DD.
func Parse(s string) (Date, error) {
	t, err := time.Parse(layout, s)
	if err != nil {
		return Date{}, fmt.Errorf("date invalide %q : format attendu AAAA-MM-JJ", s)
	}
	return Date{t}, nil
}

// String retourne la date au format YYYY-MM-DD.
func (d Date) String() string { return d.Format(layout) }

// MarshalJSON implémente json.Marshaler.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON implémente json.Unmarshaler.
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("date invalide : chaîne AAAA-MM-JJ attendue")
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Scan implémente sql.Scanner.
func (d *Date) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		d.Time = v
	case string:
		parsed, err := Parse(v)
		if err != nil {
			return err
		}
		*d = parsed
	case []byte:
		parsed, err := Parse(string(v))
		if err != nil {
			return err
		}
		*d = parsed
	default:
		return fmt.Errorf("civil.Date : type non géré %T", src)
	}
	return nil
}

// Value implémente driver.Valuer.
func (d Date) Value() (driver.Value, error) { return d.Time, nil }
