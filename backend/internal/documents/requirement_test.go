package documents

import "testing"

func TestIsRequiredFollowsTheSlotRequirementAndTheCustomerType(t *testing.T) {
	cases := []struct {
		requirement, customerType string
		want                      bool
	}{
		{"always", "diplomatico", true},
		{"always", "embajada", true},
		{"conditional", "diplomatico", false}, // the power of attorney is not needed for a Diplomático
		{"conditional", "embajada", true},
		{"conditional", "entidad_religiosa_catolica", true},
		{"conditional", "organismo_internacional", true},
	}
	for _, c := range cases {
		if got := slotRequired(c.requirement, c.customerType); got != c.want {
			t.Errorf("isRequired(%s, %s) = %v, want %v", c.requirement, c.customerType, got, c.want)
		}
	}
}
