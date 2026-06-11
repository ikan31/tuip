package uptimekuma

import (
	"testing"

	"github.com/ikan31/tuip/internal/status"
)

func TestMapHeartbeatStatus(t *testing.T) {
	t.Parallel()

	tests := map[int]status.State{
		0:  status.StateMajorOutage,
		1:  status.StateOperational,
		2:  status.StateUnknown,
		3:  status.StateMaintenance,
		99: status.StateUnknown,
	}

	for value, want := range tests {
		if got := mapHeartbeatStatus(value); got != want {
			t.Fatalf("mapHeartbeatStatus(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestMapComponentsAggregatesWorstState(t *testing.T) {
	t.Parallel()

	groups := []publicGroup{
		{
			Name: "Core",
			MonitorList: []monitor{
				{ID: 1, Name: "API"},
				{ID: 2, Name: "Web"},
			},
		},
	}
	heartbeats := map[string][]heartbeat{
		"1": {{Status: 1, Time: "2026-06-11 03:16:35.926"}},
		"2": {{Status: 0, Time: "2026-06-11 03:17:35.926"}},
	}

	components, state, updatedAt := mapComponents(groups, heartbeats)
	if state != status.StateMajorOutage {
		t.Fatalf("state = %q, want %q", state, status.StateMajorOutage)
	}

	if len(components) != 2 {
		t.Fatalf("len(components) = %d, want 2", len(components))
	}

	if components[1].Status != "down" {
		t.Fatalf("components[1].Status = %q, want down", components[1].Status)
	}

	if updatedAt == nil {
		t.Fatal("updatedAt is nil, want latest heartbeat time")
	}
}
