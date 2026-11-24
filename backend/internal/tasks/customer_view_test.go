package tasks

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// These tests are the guard on what customers can read. Visibility is an allowlist; here every field of every
// task's data is classified, and the build of the customer view is checked against that classification with
// recognizable values, so an internal value cannot reach a customer and a new field cannot slip through.

type classification struct {
	visible    []string // shown to customers
	calculated []string // computed values shown to customers
	internal   []string // never shown
}

// customerClassification is the specification's list (section 5.1) field by field.
var customerClassification = map[int]classification{
	1:  {visible: []string{"initialToyotaAmount", "initialCommission", "accessoryAmount", "releaseProcessingAmount"}, calculated: []string{"quotedFinalPrice"}},
	2:  {visible: []string{"signerDocumentType", "signerDocumentNumber", "signerFirstName", "signerLastName"}, internal: []string{"signerNameSource", "pepStatus", "ddcRegime"}},
	3:  {visible: []string{"vehicleSeries", "vin", "finalToyotaAmount"}, calculated: []string{"finalCommission"}},
	4:  {visible: []string{"bank", "paymentDate", "transactionNumber", "depositedAmount"}, internal: []string{"mismatchReason"}},
	5:  {internal: []string{"bank", "paymentDate", "transactionNumber", "depositedAmount", "mismatchReason"}},
	6:  {visible: invoiceKeys("number", "date", "amount"), internal: invoiceKeys("mismatchReason")},
	7:  {},
	8:  {internal: []string{"damNumber", "customsOffice", "controlChannel"}},
	9:  {internal: []string{"personInCharge"}},
	10: {visible: []string{"licensePlate", "registrationDate"}},
	11: {visible: []string{"pdsTransferDate"}},
	12: {visible: []string{"equipmentCompletionDate"}, internal: []string{"serviceOrderNumber"}},
	13: {visible: []string{"deliveryDate"}, internal: []string{"surquilloTransferDate"}},
}

func invoiceKeys(fields ...string) []string {
	var keys []string
	for _, kind := range []string{"toyota", "commission", "accessories", "releaseProcessing"} {
		for _, f := range fields {
			keys = append(keys, kind+"."+f)
		}
	}
	return keys
}

func blankData(task int) any {
	switch task {
	case 1:
		return &Task1Data{}
	case 2:
		return &Task2Data{}
	case 3:
		return &Task3Data{}
	case 4, 5:
		return &PaymentData{}
	case 6:
		return &Task6Data{}
	case 7:
		return &Task7Data{}
	case 8:
		return &Task8Data{}
	case 9:
		return &Task9Data{}
	case 10:
		return &Task10Data{}
	case 11:
		return &Task11Data{}
	case 12:
		return &Task12Data{}
	}
	return &Task13Data{}
}

// jsonName is the name a struct field has in JSON.
func jsonName(f reflect.StructField) string { return strings.Split(f.Tag.Get("json"), ",")[0] }

// fill puts a recognizable value into every field and returns the key of each, with its value.
func fill(v reflect.Value, prefix string, out map[string]string) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		name := prefix + jsonName(t.Field(i))
		field := v.Field(i)
		switch field.Type().Kind() {
		case reflect.Ptr:
			if field.Type().Elem().Kind() == reflect.String {
				value := "SENTINEL<" + name + ">"
				field.Set(reflect.ValueOf(&value))
				out[name] = value
			} else { // a nested struct, such as an invoice
				nested := reflect.New(field.Type().Elem())
				fill(nested.Elem(), name+".", out)
				field.Set(nested)
			}
		}
	}
}

func sorted(keys []string) []string {
	out := slices.Clone(keys)
	sort.Strings(out)
	return out
}

func TestEveryFieldOfEveryTaskIsClassifiedForCustomers(t *testing.T) {
	for task := 1; task <= TaskCount; task++ {
		fields := map[string]string{}
		fill(reflect.ValueOf(blankData(task)).Elem(), "", fields)
		var found []string
		for key := range fields {
			found = append(found, key)
		}
		c := customerClassification[task]
		want := append(slices.Clone(c.visible), c.internal...)
		if !slices.Equal(sorted(found), sorted(want)) {
			t.Errorf("task %d: the fields of its data are %v but the customer classification covers %v: classify every field as visible or internal",
				task, sorted(found), sorted(want))
		}
	}
}

func TestCustomersSeeExactlyTheVisibleFieldsAndNothingInternal(t *testing.T) {
	cents := func(v int64) *int64 { return &v }
	f := fin{
		initialToyota: cents(1), initialCommission: cents(2), accessory: cents(3), release: cents(4),
		quoted: cents(1000), finalToyota: cents(5), finalCommission: cents(6),
	}

	for task := 1; task <= TaskCount; task++ {
		values := map[string]string{}
		data := blankData(task)
		fill(reflect.ValueOf(data).Elem(), "", values)
		c := customerClassification[task]

		shown := map[string]string{}
		for _, field := range customerFields(task, data, f) {
			shown[field.Key] = field.Value
		}

		var keys []string
		for key := range shown {
			keys = append(keys, key)
		}
		want := append(slices.Clone(c.visible), c.calculated...)
		if !slices.Equal(sorted(keys), sorted(want)) {
			t.Errorf("task %d shows %v to customers, want exactly %v", task, sorted(keys), sorted(want))
		}
		for _, key := range c.visible {
			if shown[key] != values[key] {
				t.Errorf("task %d: %s must reach the customer unchanged (got %q, want %q)", task, key, shown[key], values[key])
			}
		}
		for _, key := range c.internal {
			for shownKey, shownValue := range shown {
				if strings.Contains(shownValue, values[key]) {
					t.Errorf("task %d: the internal value of %s leaked through %s", task, key, shownKey)
				}
			}
		}
		if (len(c.visible)+len(c.calculated) > 0) != showsDataToCustomers[task] {
			t.Errorf("task %d: showsDataToCustomers disagrees with the classification", task)
		}
	}
}

func TestCustomersSeeNoValueThatIsNotRecordedYet(t *testing.T) {
	for task := 1; task <= TaskCount; task++ {
		if fields := customerFields(task, blankData(task), fin{}); len(fields) != 0 {
			t.Errorf("task %d shows %d fields with nothing recorded", task, len(fields))
		}
	}
}
