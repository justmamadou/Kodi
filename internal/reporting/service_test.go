package reporting

import "testing"

func TestFormatFCFA(t *testing.T) {
	tests := map[int64]string{
		0: "0", 999: "999", 1000: "1 000", 1155000: "1 155 000", -25000: "-25 000",
	}
	for in, want := range tests {
		if got := FormatFCFA(in); got != want {
			t.Errorf("FormatFCFA(%d) = %q, attendu %q", in, got, want)
		}
	}
}

func TestSanitizeCellBlocksFormulaInjection(t *testing.T) {
	for _, dangerous := range []string{"=HYPERLINK(\"http://x\")", "+1+1", "-2", "@SUM(A1)"} {
		if got := sanitizeCell(dangerous); got != "'"+dangerous {
			t.Errorf("sanitizeCell(%q) = %q", dangerous, got)
		}
	}
	if got := sanitizeCell("Ciment 50 sacs"); got != "Ciment 50 sacs" {
		t.Errorf("un texte normal ne doit pas changer : %q", got)
	}
	if got := sanitizeCell(""); got != "" {
		t.Errorf("une cellule vide reste vide : %q", got)
	}
}
