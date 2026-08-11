package signal

import (
	"strings"
	"testing"
)

func TestRelayCandidateIsRejected(t *testing.T) {
	description := Description{Type: "offer", SDP: "v=0\r\na=candidate:1 1 UDP 1 203.0.113.1 5000 typ relay\r\n"}
	if err := ValidateDescription(description, "offer"); err == nil {
		t.Fatal("relay candidate was accepted")
	}
}

func TestCandidateFloodIsRejected(t *testing.T) {
	lines := []string{"v=0"}
	for index := 0; index <= MaxCandidateLines; index++ {
		lines = append(lines, "a=candidate:1 1 UDP 1 192.0.2.1 5000 typ host")
	}
	if err := ValidateDescription(Description{Type: "answer", SDP: strings.Join(lines, "\r\n")}, "answer"); err == nil {
		t.Fatal("candidate flood was accepted")
	}
}
