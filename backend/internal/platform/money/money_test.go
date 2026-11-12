package money

import "testing"

func TestParse(t *testing.T) {
	good := map[string]int64{"0": 0, "1": 100, "1.5": 150, "0.01": 1, "35000.00": 3500000, " 12.30 ": 1230, "9999999999.99": MaxCents}
	for raw, want := range good {
		if got, ok := Parse(raw); !ok || got != want {
			t.Errorf("Parse(%q) = %d, %v; want %d", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"", ".5", "1.", "1.234", "-1", "+1", "1e3", "1,000", "12345678901", "1 000", "٣", "10000000000.00"} {
		if _, ok := Parse(raw); ok {
			t.Errorf("Parse(%q) must fail", raw)
		}
	}
}

func TestFormat(t *testing.T) {
	for cents, want := range map[int64]string{0: "0.00", 5: "0.05", 150: "1.50", 3500000: "35000.00", -250: "-2.50"} {
		if got := Format(cents); got != want {
			t.Errorf("Format(%d) = %s, want %s", cents, got, want)
		}
	}
}
