package managedexec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// IAM-012 asks for a deterministic answer to "what happens when a membership
// changes while the provider is already carrying work". The answer this
// platform gives is: the organization's decision blocks NEW work and does not
// touch work already in custody — the parcel is in the vehicle, the receiver
// is waiting, and refusing the delivery step would strand goods with nobody
// accountable for them.
//
// That answer is only true while the corporate gate stays on the acquisition
// paths. A well-meaning addition of the same check to a lifecycle step would
// silently reverse the policy, and no behavioural test would notice, because
// the reversal only shows up for a provider whose membership changes mid-trip.
// So the placement is asserted directly.
func TestCorporateGateStaysOnAcquisitionPathsOnly(t *testing.T) {
	// Every file that may call the managed-membership gate. Each one is a
	// decision to START work: an eligibility answer, an execution creation,
	// or a response to an offer or assignment.
	acquisition := map[string]bool{
		filepath.Join("managedexec", "service.go"):   true,
		filepath.Join("fleetprovider", "http.go"):    true,
		filepath.Join("fleetprovider", "service.go"): true,
	}

	root := filepath.Join("..", "..", "features")
	var offenders []string
	found := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		source := string(body)
		if !strings.Contains(source, "AuthorizeManagedOperation") &&
			!strings.Contains(source, "VerifyManagedContext") {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		found++
		if !acquisition[relative] {
			offenders = append(offenders, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk features: %v", err)
	}
	if found == 0 {
		t.Fatal("no file calls the corporate membership gate at all; either the " +
			"walk is looking in the wrong place or managed work is no longer gated")
	}
	if len(offenders) > 0 {
		t.Fatalf("the corporate membership gate reached a path that is not about "+
			"starting work: %v\n\nIf one of these really is an acquisition path, add it to "+
			"the list above. If it is a lifecycle step, moving the gate there means a "+
			"provider suspended mid-trip can no longer complete a delivery they are "+
			"already carrying, which is the outcome IAM-012 forbids.", offenders)
	}
}
