package decimal

import "testing"

func TestDecimalStringRoundsToFourPlaces(t *testing.T) {
	tests := map[string]string{
		"333.33":    "333.3300",
		"0.00004":   "0.0000",
		"0.00005":   "0.0001",
		"18.88888":  "18.8889",
		"59.800000": "59.8000",
	}
	for input, want := range tests {
		got, err := Parse(input)
		if err != nil {
			t.Fatalf("parse %s: %v", input, err)
		}
		if got.String() != want {
			t.Fatalf("Parse(%q).String()=%s, want %s", input, got.String(), want)
		}
	}
}

func TestDecimalParseRejectsNonDecimalSyntax(t *testing.T) {
	for _, input := range []string{"1/3", "1e-3", "NaN", "Infinity"} {
		if _, err := Parse(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}

func TestDecimalIsIntegerUsesExactValue(t *testing.T) {
	tests := map[string]bool{
		"1":       true,
		"1.0000":  true,
		"1.00001": false,
		"0.5":     false,
	}
	for input, want := range tests {
		got, err := Parse(input)
		if err != nil {
			t.Fatalf("parse %s: %v", input, err)
		}
		if got.IsInteger() != want {
			t.Fatalf("Parse(%q).IsInteger()=%v, want %v", input, got.IsInteger(), want)
		}
	}
}
