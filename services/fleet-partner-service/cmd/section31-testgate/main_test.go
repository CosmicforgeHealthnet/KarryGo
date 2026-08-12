package main

import (
	"strings"
	"testing"
)

func TestGateAcceptsPassStreamWithNoSkips(t *testing.T) {
	stream := strings.NewReader("{\"Action\":\"run\",\"Package\":\"example\",\"Test\":\"TestMandatory\"}\n{\"Action\":\"pass\",\"Package\":\"example\",\"Test\":\"TestMandatory\"}\n")
	if err := enforce(stream, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGateRejectsUnexpectedMandatorySkip(t *testing.T) {
	stream := strings.NewReader("{\"Action\":\"skip\",\"Package\":\"example\",\"Test\":\"TestMandatory\"}\n")
	if err := enforce(stream, nil); err == nil {
		t.Fatal("unexpected skip was accepted")
	}
}

func TestGateAllowsNamedHarnessSkip(t *testing.T) {
	stream := strings.NewReader("{\"Action\":\"skip\",\"Package\":\"example\",\"Test\":\"TestHarnessOnly\"}\n")
	if err := enforce(stream, []string{"TestHarnessOnly"}); err != nil {
		t.Fatal(err)
	}
}

func TestGateRejectsFailedTest(t *testing.T) {
	stream := strings.NewReader("{\"Action\":\"run\",\"Package\":\"example\",\"Test\":\"TestMandatory\"}\n{\"Action\":\"fail\",\"Package\":\"example\",\"Test\":\"TestMandatory\"}\n")
	if err := enforce(stream, nil); err == nil {
		t.Fatal("failed test was accepted")
	}
}
