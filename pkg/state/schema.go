package state

import (
	"encoding/json"
	"time"
)

// State specifies a chall-manager state as it will be stored and propagated
// across instances.
type State struct {
	// The opaque JSON Pulumi state.
	Pulumi json.RawMessage `json:"pulumi"`

	// Metadata of the current state. It holds information for other μServices
	// or necessary information to hold consistency.
	Metadata StateMetadata `json:"metadata"`

	// Outputs of the state once deployed (or renewed).
	Outputs StateOutputs `json:"outputs"`
}

type StateMetadata struct {
	// ChallengeId is has been instanciated from.
	ChallengeId string `json:"challenge_id"`

	// Source holds the directory structure of the challenge it is instanciated from.
	Source string `json:"source"`

	// Until holds both the timeout and until concepts of the API.
	// If "until" is given, use it straightfully, if "timeout" is given,
	// compute until = now + timeout.
	// If nil, represent an infinite-duration resource, but could be stopped manually.
	Until *time.Time `json:"until,omitempty"`
}

type StateOutputs struct {
	// ConnectionInfo to the Challenge Scenario on Demand.
	ConnectionInfo string `json:"connection_info"`
	// Flag specific to the Challenge Scenario on Demand.
	// Avoid shareflag.
	Flag *string `json:"flag,omitempty"`
}
