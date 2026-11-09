package expense

import "testing"

func TestRemainingAfter(t *testing.T) {
	tests := []struct {
		name                  string
		amount, paid, payment int64
		remaining             int64
		ok                    bool
	}{
		{"paiement partiel", 1000, 0, 400, 600, true},
		{"second paiement partiel", 1000, 400, 100, 500, true},
		{"solde exact", 1000, 400, 600, 0, true},
		{"dépassement d'une unité", 1000, 400, 601, 600, false},
		{"déjà soldée", 1000, 1000, 1, 0, false},
		{"montant nul", 1000, 0, 0, 1000, false},
		{"montant négatif", 1000, 0, -5, 1000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RemainingAfter(tt.amount, tt.paid, tt.payment)
			if got != tt.remaining || ok != tt.ok {
				t.Fatalf("RemainingAfter = (%d, %v), attendu (%d, %v)", got, ok, tt.remaining, tt.ok)
			}
		})
	}
}
