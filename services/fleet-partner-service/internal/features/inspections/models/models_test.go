package models

import (
	"errors"
	"testing"
)

func TestEvaluateInspectionResults(t *testing.T) {
	items := []ChecklistItem{{Code: "brakes", Required: true, Critical: true}, {Code: "lights", Required: true}, {Code: "wipers"}, {Code: "photo", EvidenceRequired: true}}
	cases := []struct {
		answers []Answer
		want    string
		err     error
	}{
		{[]Answer{{ItemCode: "brakes", Passed: true}, {ItemCode: "lights", Passed: true}}, ResultPass, nil},
		{[]Answer{{ItemCode: "brakes", Passed: true}, {ItemCode: "lights", Passed: true, Advisory: true}}, ResultPassWithAdvisory, nil},
		{[]Answer{{ItemCode: "brakes", Passed: true}, {ItemCode: "lights", Passed: false}}, ResultFail, nil},
		{[]Answer{{ItemCode: "brakes", Passed: false}, {ItemCode: "lights", Passed: false}}, ResultCriticalFail, nil},
		{[]Answer{{ItemCode: "brakes", Passed: true}}, "", ErrMandatoryAnswerMissing},
		{[]Answer{{ItemCode: "brakes", Passed: true}, {ItemCode: "lights", Passed: true}, {ItemCode: "photo", Passed: true}}, "", ErrEvidenceMissing},
	}
	for i, c := range cases {
		got, err := Evaluate(items, c.answers, nil)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("case %d: got %q %v, want %q %v", i, got, err, c.want, c.err)
		}
	}
}
