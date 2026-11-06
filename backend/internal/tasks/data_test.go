package tasks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestEveryTaskHasADataHandler(t *testing.T) {
	for n := 1; n <= LastTask; n++ {
		if kindFor(n) == nil {
			t.Errorf("task %d has no data handler", n)
		}
	}
	if kindFor(0) != nil || kindFor(14) != nil {
		t.Error("only tasks 1 to 13 exist")
	}
}

func TestRequiredSlots(t *testing.T) {
	has := func(slots []string, want string) bool {
		for _, s := range slots {
			if s == want {
				return true
			}
		}
		return false
	}
	if has(requiredSlots(2, "diplomatico"), "power_of_attorney") {
		t.Error("a Diplomático needs no power of attorney")
	}
	for _, customerType := range []string{"embajada", "entidad_religiosa_catolica", "organismo_internacional"} {
		if !has(requiredSlots(2, customerType), "power_of_attorney") {
			t.Errorf("%s needs a power of attorney", customerType)
		}
	}
	for _, n := range []int{9, 11} {
		if got := requiredSlots(n, "embajada"); got == nil || len(got) != 0 {
			t.Errorf("task %d needs no document and must list an empty array, got %v", n, got)
		}
	}
	if got := requiredSlots(6, "embajada"); len(got) != 4 {
		t.Errorf("task 6 needs its four invoices, got %v", got)
	}
}

// Unknown or calculated keys are refused before the database is touched, so these need no connection.
func TestTableKindRefusesUnknownAndCalculatedKeys(t *testing.T) {
	sale := testSale()
	ctx := context.Background()

	_, err := task1.save(ctx, nil, sale, json.RawMessage(`{"nope": 1}`), uuid.Nil, testNow(), nil)
	if err == nil {
		t.Fatal("an unknown key must be refused")
	}
	_, err = task1.save(ctx, nil, sale, json.RawMessage(`{"quotedFinalPrice": "1.00"}`), uuid.Nil, testNow(), nil)
	if err == nil {
		t.Fatal("a calculated field must be refused")
	}
}

func TestTask7AcceptsOnlyAnEmptyBody(t *testing.T) {
	sale := testSale()
	if _, err := (task7{}).save(context.Background(), nil, sale, json.RawMessage(`{}`), uuid.Nil, testNow(), nil); err != nil {
		t.Fatalf("empty body: %v", err)
	}
	if _, err := (task7{}).save(context.Background(), nil, sale, json.RawMessage(`{"x": 1}`), uuid.Nil, testNow(), nil); err == nil {
		t.Fatal("task 7 has no fields")
	}
}

func TestCleanReason(t *testing.T) {
	if _, err := cleanReason("   "); err == nil {
		t.Error("a blank reason must be refused")
	}
	long := make([]rune, 501)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := cleanReason(string(long)); err == nil {
		t.Error("a 501 character reason must be refused")
	}
	if got, err := cleanReason("  error de digitación "); err != nil || got != "error de digitación" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestCheckNumber(t *testing.T) {
	for _, n := range []int{0, -1, 14} {
		if checkNumber(n) == nil {
			t.Errorf("task %d must be refused", n)
		}
	}
	if checkNumber(1) != nil || checkNumber(13) != nil {
		t.Error("tasks 1 and 13 exist")
	}
}
