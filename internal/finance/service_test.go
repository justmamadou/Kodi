package finance

import (
	"testing"
	"time"
)

func TestAssess(t *testing.T) {
	thresholds := []int16{80, 90, 100}
	tests := []struct {
		name             string
		budget, approved int64
		health           string
		reached          int16 // 0 = aucun
		overrun          int64
	}{
		{"sans budget ni dépense", 0, 0, HealthNoBudget, 0, 0},
		{"dépense sans budget publié", 0, 500, HealthOver, 0, 500},
		{"sous les seuils", 1000, 799, HealthOK, 0, 0},
		{"seuil de 80 atteint pile", 1000, 800, HealthWarning, 80, 0},
		{"seuil de 90", 1000, 950, HealthWarning, 90, 0},
		{"budget entièrement consommé", 1000, 1000, HealthWarning, 100, 0},
		{"dépassement d'une unité", 1000, 1001, HealthOver, 100, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health, _, reached, overrun := Assess(tt.budget, tt.approved, thresholds)
			if health != tt.health || overrun != tt.overrun {
				t.Fatalf("health=%s overrun=%d, attendu %s/%d", health, overrun, tt.health, tt.overrun)
			}
			var got int16
			if reached != nil {
				got = *reached
			}
			if got != tt.reached {
				t.Fatalf("seuil atteint = %d, attendu %d", got, tt.reached)
			}
		})
	}
}

func TestNewFiguresKeepsAmountsDistinct(t *testing.T) {
	f := newFigures(1000, 600, 200, 250)
	if f.Remains != 400 {
		t.Errorf("le reste doit ignorer les dépenses en attente : %d", f.Remains)
	}
	if f.Unpaid != 350 {
		t.Errorf("approuvé non payé = %d", f.Unpaid)
	}
}

func TestSignal(t *testing.T) {
	tests := []struct {
		name             string
		budget, approved int64
		progress         int16
		want             string
	}{
		{"sans budget ni dépense", 0, 0, 0, SignalNone},
		{"dépense sans budget publié", 0, 100, 0, SignalCritical},
		{"maquette Keur Massar : 95 % pour 80 %", 100, 95, 80, SignalCritical},
		{"maquette Almadies : 58 % pour 45 %", 100, 58, 45, SignalWatch},
		{"maquette Plateau : 55 % pour 60 %", 100, 55, 60, SignalGood},
		{"maquette Mermoz : 40 % pour 35 %", 100, 40, 35, SignalGood},
		{"écart de 10 points exactement, toléré", 100, 60, 50, SignalGood},
		{"écart de 11 points", 100, 61, 50, SignalWatch},
		{"seuil élevé mais avancement en avance", 100, 92, 95, SignalGood},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Signal(tt.budget, tt.approved, tt.progress); got != tt.want {
				t.Fatalf("Signal = %s, attendu %s", got, tt.want)
			}
		})
	}
}

func TestPeriodStart(t *testing.T) {
	thursday := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	if got := PeriodStart("week", thursday).String(); got != "2026-11-02" {
		t.Fatalf("lundi attendu : %s", got)
	}
	if got := PeriodStart("month", thursday).String(); got != "2026-11-01" {
		t.Fatalf("1er du mois attendu : %s", got)
	}
	if PeriodStart("all", thursday) != nil {
		t.Fatal("aucune borne attendue pour Tout")
	}
	sunday := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	if got := PeriodStart("week", sunday).String(); got != "2026-11-02" {
		t.Fatalf("le dimanche appartient à la semaine commencée le lundi : %s", got)
	}
}

func TestWeekDays(t *testing.T) {
	days := WeekDays(time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC))
	if len(days) != 6 || days[0].String() != "2026-11-02" || days[5].String() != "2026-11-07" {
		t.Fatalf("lundi à samedi attendus : %v", days)
	}
}
