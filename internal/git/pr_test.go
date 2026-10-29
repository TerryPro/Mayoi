package git

import (
	"errors"
	"testing"
)

func TestParsePR(t *testing.T) {
	pr, err := parsePR([]byte(`{"number":123,"state":"OPEN","url":"https://github.com/o/r/pull/123","reviewDecision":"APPROVED",
		"statusCheckRollup":[
			{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"},
			{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SKIPPED"},
			{"__typename":"CheckRun","status":"IN_PROGRESS","conclusion":""},
			{"__typename":"StatusContext","state":"SUCCESS"}]}`))
	if err != nil || pr.Number != 123 || pr.Review != "APPROVED" || pr.URL != "https://github.com/o/r/pull/123" {
		t.Fatalf("pr = %+v, %v", pr, err)
	}
	if pr.Checks != ChecksPending || pr.Passed != 3 || pr.Pending != 1 || pr.Failed != 0 {
		t.Fatalf("checks = %+v", pr)
	}
	pr, _ = parsePR([]byte(`{"number":1,"statusCheckRollup":[{"__typename":"CheckRun","status":"IN_PROGRESS"},{"__typename":"StatusContext","state":"ERROR"}]}`))
	if pr.Checks != ChecksFail || pr.Failed != 1 {
		t.Fatalf("failed = %+v", pr)
	}
	pr, _ = parsePR([]byte(`{"number":1,"statusCheckRollup":[{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"}]}`))
	if pr.Checks != ChecksPass {
		t.Fatalf("passed = %+v", pr)
	}
	pr, _ = parsePR([]byte(`{"number":1,"statusCheckRollup":[]}`))
	if pr.Checks != ChecksNone {
		t.Fatalf("none = %+v", pr)
	}
	if _, err := parsePR([]byte(`{}`)); !errors.Is(err, ErrNoPR) {
		t.Fatalf("empty = %v", err)
	}
}
