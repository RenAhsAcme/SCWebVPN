package signal

import (
	"errors"
	"strings"
)

const (
	MaxSDPSize        = 512 << 10
	MaxCandidateLines = 128
)

type Description struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

func ValidateDescription(description Description, expectedType string) error {
	if description.Type != expectedType {
		return errors.New("unexpected session description type")
	}
	if len(description.SDP) == 0 || len(description.SDP) > MaxSDPSize || strings.ContainsRune(description.SDP, '\x00') {
		return errors.New("invalid session description size or encoding")
	}
	candidates := 0
	for _, rawLine := range strings.Split(description.SDP, "\n") {
		line := strings.ToLower(strings.TrimSpace(rawLine))
		if !strings.HasPrefix(line, "a=candidate:") {
			continue
		}
		candidates++
		if candidates > MaxCandidateLines {
			return errors.New("too many ICE candidates")
		}
		if strings.Contains(" "+line+" ", " typ relay ") {
			return errors.New("relay ICE candidates are forbidden")
		}
	}
	return nil
}
