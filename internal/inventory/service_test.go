package inventory

import (
	"testing"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/shopspring/decimal"
)

func TestValidateMovement(t *testing.T) {
	date := civil.Date{}
	reason := "inventaire"
	item := func(q string) MovementItem {
		return MovementItem{MaterialID: uuid.New(), Quantity: decimal.RequireFromString(q)}
	}
	same := uuid.New()
	tests := []struct {
		name string
		in   MovementInput
		ok   bool
	}{
		{"entrée valide", MovementInput{MovementType: "in", OccurredOn: &date, Items: []MovementItem{item("10.5")}}, true},
		{"type inconnu", MovementInput{MovementType: "vol", OccurredOn: &date, Items: []MovementItem{item("1")}}, false},
		{"sans date", MovementInput{MovementType: "in", Items: []MovementItem{item("1")}}, false},
		{"sans article", MovementInput{MovementType: "in", OccurredOn: &date}, false},
		{"sortie négative", MovementInput{MovementType: "out", OccurredOn: &date, Items: []MovementItem{item("-1")}}, false},
		{"quantité nulle", MovementInput{MovementType: "in", OccurredOn: &date, Items: []MovementItem{item("0")}}, false},
		{"trop de décimales", MovementInput{MovementType: "in", OccurredOn: &date, Items: []MovementItem{item("1.2345")}}, false},
		{"ajustement sans motif", MovementInput{MovementType: "adjustment", OccurredOn: &date, Items: []MovementItem{item("-2")}}, false},
		{"ajustement négatif motivé", MovementInput{MovementType: "adjustment", OccurredOn: &date, Reason: &reason, Items: []MovementItem{item("-2")}}, true},
		{"article en double", MovementInput{MovementType: "in", OccurredOn: &date, Items: []MovementItem{
			{MaterialID: same, Quantity: decimal.NewFromInt(1)}, {MaterialID: same, Quantity: decimal.NewFromInt(2)},
		}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateMovement(tt.in); (err == nil) != tt.ok {
				t.Fatalf("erreur = %v, valide attendu = %v", err, tt.ok)
			}
		})
	}
}

func TestDelta(t *testing.T) {
	q := decimal.NewFromInt(5)
	if !Delta("in", q).Equal(q) || !Delta("adjustment", q).Equal(q) {
		t.Error("une entrée ou un ajustement conserve le signe")
	}
	if !Delta("out", q).Equal(q.Neg()) {
		t.Error("une sortie diminue le stock")
	}
}
