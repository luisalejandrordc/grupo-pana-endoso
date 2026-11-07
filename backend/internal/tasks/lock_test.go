package tasks

import (
	"encoding/json"
	"endosoflow/internal/platform/apperr"
	"sort"
	"testing"
)

func TestLockedFieldsFollowTheSpecTable(t *testing.T) {
	type c struct {
		name    string
		task    int
		highest int
		sent    []string
		want    []string
	}
	cases := []c{
		{"task 1 amounts are free while only task 1 is completed", 1, 1, []string{"initialToyotaAmount"}, nil},
		{"task 1 amounts lock when task 2 is completed", 1, 2, []string{"initialToyotaAmount", "accessoryAmount"}, []string{"accessoryAmount", "initialToyotaAmount"}},
		{
			"task 1 locks all four amounts further on", 1, 9,
			[]string{"initialToyotaAmount", "initialCommission", "accessoryAmount", "releaseProcessingAmount"},
			[]string{"accessoryAmount", "initialCommission", "initialToyotaAmount", "releaseProcessingAmount"},
		},
		{"task 3 final amount is free until task 4", 3, 3, []string{"finalToyotaAmount"}, nil},
		{"task 3 final amount locks when task 4 is completed", 3, 4, []string{"finalToyotaAmount", "vin"}, []string{"finalToyotaAmount"}},
		{"task 3 identifiers are never locked", 3, 12, []string{"vin", "vehicleSeries"}, nil},
		{"task 4 deposit is free before task 6", 4, 5, []string{"depositedAmount"}, nil},
		{"task 4 deposit locks when task 6 is completed", 4, 6, []string{"depositedAmount", "bank"}, []string{"depositedAmount"}},
		{"task 5 deposit locks when task 6 is completed", 5, 7, []string{"depositedAmount", "mismatchReason"}, []string{"depositedAmount"}},
		{"task 6 has no locked field", 6, 13, []string{"toyota"}, nil},
		{"task 2 has no locked field", 2, 13, []string{"signerFirstName"}, nil},
	}
	for _, tc := range cases {
		got := lockedFields(tc.task, tc.highest, tc.sent)
		sort.Strings(got)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			}
		}
	}
}

func TestTopLevelKeys(t *testing.T) {
	keys := topLevelKeys(json.RawMessage(`{"a": 1, "b": {"c": 2}}`))
	sort.Strings(keys)
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("keys = %v", keys)
	}
	if got := topLevelKeys(json.RawMessage(`[1,2]`)); got != nil {
		t.Fatalf("an array has no keys: %v", got)
	}
	if got := topLevelKeys(nil); got != nil {
		t.Fatalf("an empty body has no keys: %v", got)
	}
}

func TestFieldsLockedNamesEveryField(t *testing.T) {
	e, ok := fieldsLocked([]string{"initialToyotaAmount", "accessoryAmount"}).(*apperr.Error)
	if !ok || e.Code != "FIELD_LOCKED" || e.Status != 422 || len(e.Fields) != 2 {
		t.Fatalf("unexpected error: %+v", e)
	}
	if e.Fields[0].Field != "initialToyotaAmount" || e.Fields[0].Code != "LOCKED" {
		t.Fatalf("fields = %+v", e.Fields)
	}
}
