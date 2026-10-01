package booking

// Section 17 guard.
//
// A customer dispatch booking's held payment is bound to the rider who
// accepted it (HandleRequestAccepted -> AssignProvider), and CompleteJob
// releases it to that binding. That is correct today only because nothing
// ever changes a trip's provider: acceptance is a CAS from 'searching', and a
// replacement is RESERVED on the incident (replacement_provider_id) but the
// trip itself is never repointed -- every replacement query asserts
// t.provider_id = i.provider_id.
//
// The moment some code repoints a trip, the held payment would be released to
// the rider who left. This test fails at that moment, naming the fix: call
// walletclient.RebindToReplacement for the booking after the repoint commits,
// as Hauling's recovery does, and guard settlement the way Hauling's
// settleOnComplete does.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// tripSetClause captures the SET clause of an UPDATE on trips -- up to its
// WHERE, RETURNING or the end of the statement -- so a provider_id in the
// filter is not mistaken for one being assigned.
var (
	tripSetClause   = regexp.MustCompile(`(?is)update\s+trips\s+(?:\w+\s+)?set\s(.*?)(?:\bwhere\b|\breturning\b|;|` + "`" + `|$)`)
	assignsProvider = regexp.MustCompile(`(?i)(?:^|[\s,(])provider_id\s*=`)
)

type repointMatcher struct{}

func (repointMatcher) MatchString(text string) bool {
	for _, clause := range tripSetClause.FindAllStringSubmatch(text, -1) {
		if assignsProvider.MatchString(clause[1]) {
			return true
		}
	}
	return false
}

func (m repointMatcher) Match(content []byte) bool { return m.MatchString(string(content)) }

var repointsTripProvider repointMatcher

func TestNoCodeRepointsATripProviderWithoutRebindingItsPayment(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if repointsTripProvider.Match(content) && !strings.Contains(string(content), "RebindToReplacement") {
			offenders = append(offenders, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("these files change a trip's provider but never move the held payment "+
			"(walletclient.RebindToReplacement): %v", offenders)
	}
}

func TestTheGuardRecognisesARepoint(t *testing.T) {
	for _, sample := range []string{
		"UPDATE trips SET provider_id=$2 WHERE id=$1",
		"UPDATE trips t SET status='assigned', provider_id = $3",
		"update trips\n\tset updated_at=now(),\n\t\tprovider_id=$2",
	} {
		if !repointsTripProvider.MatchString(sample) {
			t.Fatalf("guard missed %q", sample)
		}
	}
	for _, sample := range []string{
		"UPDATE trips SET delivery_code='' WHERE id=$1",
		"UPDATE trips SET status='completed' WHERE provider_id=$2",
	} {
		if repointsTripProvider.MatchString(sample) {
			t.Fatalf("guard flagged a non-repoint %q", sample)
		}
	}
}
