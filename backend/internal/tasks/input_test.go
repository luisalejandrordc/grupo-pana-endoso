package tasks

import (
	"encoding/json"
	"testing"
)

func TestNormalizeInputReadsNumbersAndNulls(t *testing.T) {
	out, err := normalizeInput([]byte(`{"initialToyotaAmount": 30000, "accessoryAmount": 1200.50, "initialCommission": null, "bank": "bcp"}`))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"initialToyotaAmount": "30000", "accessoryAmount": "1200.50", "initialCommission": "", "bank": "bcp"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestNormalizeInputKeepsTask6ObjectsAndDropsNullOnes(t *testing.T) {
	out, err := normalizeInput([]byte(`{"toyota": {"number": "F001-1", "amount": 31000, "date": null}, "commission": null}`))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["commission"]; ok {
		t.Error("a null invoice means no change, so it must be dropped, not turned into text")
	}
	toyota := got["toyota"].(map[string]any)
	if toyota["amount"] != "31000" || toyota["date"] != "" || toyota["number"] != "F001-1" {
		t.Errorf("toyota = %v", toyota)
	}
}

func TestNormalizeInputRefusesWhatIsNotAnObjectOfTexts(t *testing.T) {
	for name, body := range map[string]string{
		"array":             `[1,2]`,
		"string":            `"x"`,
		"boolean field":     `{"bank": true}`,
		"array field":       `{"bank": ["a"]}`,
		"object in a field": `{"bank": {"a": "b"}}`,
		"nested too deep":   `{"toyota": {"number": {"x": "y"}}}`,
		"trailing data":     `{"bank": "bcp"} {"x": 1}`,
	} {
		if _, err := normalizeInput([]byte(body)); err == nil {
			t.Errorf("%s: must be refused", name)
		}
	}
	if out, err := normalizeInput(nil); err != nil || string(out) != "{}" {
		t.Errorf("an empty body is an empty object: %q %v", out, err)
	}
}
