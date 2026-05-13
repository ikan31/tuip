package status

import "time"

// State is tuip's normalized health state. Every provider maps its upstream
// source-specific status into one of these values.
type State string

const (
	StateOperational   State = "operational"
	StateDegraded      State = "degraded"
	StatePartialOutage State = "partial_outage"
	StateMajorOutage   State = "major_outage"
	StateMaintenance   State = "maintenance"
	StateUnknown       State = "unknown"
	StateError         State = "error"
)

// Display returns a human-readable representation of a normalized state.
func (s State) Display() string {
	switch s {
	case StateOperational:
		return "Operational"
	case StateDegraded:
		return "Degraded"
	case StatePartialOutage:
		return "Partial Outage"
	case StateMajorOutage:
		return "Major Outage"
	case StateMaintenance:
		return "Maintenance"
	case StateError:
		return "Error"
	case StateUnknown:
		fallthrough
	default:
		return "Unknown"
	}
}

// IsHealthy reports whether the provider is considered healthy for default
// human CLI behavior. A degraded SaaS service does not mean tuip failed, but
// this helper is useful for optional automation flags like --fail-on-degraded.
func (s State) IsHealthy() bool {
	return s == StateOperational
}

// IsRuntimeFailure reports whether the state represents tuip failing to check
// a provider rather than a successfully fetched upstream health problem.
func (s State) IsRuntimeFailure() bool {
	return s == StateError
}

// Snapshot is the normalized status result returned by every provider.
type Snapshot struct {
	ProviderID string      `json:"provider_id"`
	Name       string      `json:"name"`
	State      State       `json:"state"`
	Summary    string      `json:"summary"`
	SourceURL  string      `json:"source_url"`
	CheckedAt  time.Time   `json:"checked_at"`
	UpdatedAt  *time.Time  `json:"updated_at,omitempty"`
	Incidents  []Incident  `json:"incidents"`
	Components []Component `json:"components"`
	Error      string      `json:"error,omitempty"`
}

// Incident represents an active incident, scheduled maintenance item, or other
// provider timeline item that is worth showing in detailed output.
type Incident struct {
	Kind           string     `json:"kind"` // incident, maintenance, or update
	Name           string     `json:"name"`
	Status         string     `json:"status,omitempty"`
	Impact         string     `json:"impact,omitempty"`
	Summary        string     `json:"summary,omitempty"`
	URL            string     `json:"url,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	ScheduledFor   *time.Time `json:"scheduled_for,omitempty"`
	ScheduledUntil *time.Time `json:"scheduled_until,omitempty"`
}

// Component represents an optional lower-level provider component status.
type Component struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	State  State  `json:"state"`
	Group  string `json:"group,omitempty"`
}

// Response is the top-level JSON/human rendering model for a status command.
type Response struct {
	CheckedAt time.Time  `json:"checked_at"`
	Results   []Snapshot `json:"results"`
}
