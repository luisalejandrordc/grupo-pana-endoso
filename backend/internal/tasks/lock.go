package tasks

import "encoding/json"

// lockRule says which fields of a task feed calculations or checks in later tasks, and from which completed
// task on they can no longer be edited (state machines spec, section 2.4).
type lockRule struct {
	keys         []string
	lockedAtTask int // locked once a task with this number or a higher one is completed
}

// lockRules lists the financial source fields. Once any later task is completed the Manager or Admin must roll
// back in reverse order (reopening the last completed task repeatedly) before correcting them.
//
//	Task 1: the four amounts (the quoted final price is calculated from them): locked once Task 2 is completed.
//	Task 3: the final Toyota amount (the final commission is calculated from it): locked once Task 4 is completed.
//	Task 4 and Task 5: the deposited amount: locked once Task 6 is completed.
var lockRules = map[int]lockRule{
	1: {keys: []string{"initialToyotaAmount", "initialCommission", "accessoryAmount", "releaseProcessingAmount"}, lockedAtTask: 2},
	3: {keys: []string{"finalToyotaAmount"}, lockedAtTask: 4},
	4: {keys: []string{"depositedAmount"}, lockedAtTask: 6},
	5: {keys: []string{"depositedAmount"}, lockedAtTask: 6},
}

// lockedFields returns the keys of an edit that are locked, given the highest completed task of the sale.
func lockedFields(number, highestCompleted int, sentKeys []string) []string {
	rule, ok := lockRules[number]
	if !ok || highestCompleted < rule.lockedAtTask {
		return nil
	}
	locked := map[string]bool{}
	for _, k := range rule.keys {
		locked[k] = true
	}
	var out []string
	for _, k := range sentKeys {
		if locked[k] {
			out = append(out, k)
		}
	}
	return out
}

// topLevelKeys lists the keys of a JSON object in the order the encoder gives them. A body that is not an
// object has none: the task's own save refuses it afterwards.
func topLevelKeys(raw json.RawMessage) []string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	return keys
}
