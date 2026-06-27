package cli

// agf6_output_assertions_test.go — opportunistic Slice-2 follow-up (AG-F6).
// The Slice-1 validation surfaced two outputs that lacked snapshot/assertion
// tests: (a) the `guide` "installed" footer advertises `/harness`, and (b)
// `example` lists the embedded `_pack-skeleton` pack files. These are PURE
// output assertions on the existing guide.go / example.go code; they do not
// modify the source unless a test reveals an actual defect.

import (
	"bytes"
	"strings"
	"testing"
)

// TestAGF6_GuideFooterAdvertisesHarnessCommand confirms writeGuide always
// prints the `/harness` recipe pointer in its footer (the agent-operability
// contract: an agent reading `guide` learns the add-an-agent recipe exists).
func TestAGF6_GuideFooterAdvertisesHarnessCommand(t *testing.T) {
	var buf bytes.Buffer
	st := harnessState{Phase: phaseGreenfield, ProjectRoot: "/x"}
	writeGuide(&buf, st, nextSteps(st))
	if !strings.Contains(buf.String(), "/harness") {
		t.Errorf("guide footer must advertise /harness\n--- output ---\n%s", buf.String())
	}
}

// TestAGF6_GuideInstalledFooterAdvertisesHarnessCommand confirms the same for
// the installed-phase footer (the phase an operating agent most often sees).
func TestAGF6_GuideInstalledFooterAdvertisesHarnessCommand(t *testing.T) {
	var buf bytes.Buffer
	st := harnessState{Phase: phaseInstalled, ProjectRoot: "/x", HasMission: true, RuntimeBackend: "host-shell"}
	writeGuide(&buf, st, nextSteps(st))
	if !strings.Contains(buf.String(), "/harness") {
		t.Errorf("installed guide footer must advertise /harness\n--- output ---\n%s", buf.String())
	}
}

// TestAGF6_ExampleListsPackSkeleton confirms `vh-agent-harness example` (no
// arg) lists the embedded _pack-skeleton overlay pack files, so an agent knows
// a pack skeleton is available to copy.
func TestAGF6_ExampleListsPackSkeleton(t *testing.T) {
	cmd, buf := newOutCmd()
	if err := runExample(cmd, nil); err != nil {
		t.Fatalf("runExample: %v", err)
	}
	if !strings.Contains(buf.String(), "_pack-skeleton") {
		t.Errorf("example list must mention _pack-skeleton\n--- output ---\n%s", buf.String())
	}
}
