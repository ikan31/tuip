package statusio

import (
	"testing"

	"github.com/ikan31/tuip/internal/status"
)

func TestMapStatusCodes(t *testing.T) {
	t.Parallel()

	tests := map[int]status.State{
		100: status.StateOperational,
		200: status.StateMaintenance,
		300: status.StateDegraded,
		400: status.StatePartialOutage,
		500: status.StateMajorOutage,
		999: status.StateUnknown,
	}

	for code, want := range tests {
		if got := MapStatus("", code); got != want {
			t.Fatalf("MapStatus(%q, %d) = %q, want %q", "", code, got, want)
		}
	}
}

func TestMapStatusLabels(t *testing.T) {
	t.Parallel()

	tests := map[string]status.State{
		"Operational":                status.StateOperational,
		"Planned Maintenance":        status.StateMaintenance,
		"Degraded Performance":       status.StateDegraded,
		"Partial Service Disruption": status.StatePartialOutage,
		"Service Outage":             status.StateMajorOutage,
	}

	for label, want := range tests {
		if got := MapStatus(label, 0); got != want {
			t.Fatalf("MapStatus(%q, %d) = %q, want %q", label, 0, got, want)
		}
	}
}

func TestMapComponents(t *testing.T) {
	t.Parallel()

	components := mapComponents([]componentResponse{
		{
			Name:       "Cloud Orchestration",
			Status:     "Operational",
			StatusCode: 100,
			Containers: []containerResponse{
				{Name: "Prefect Cloud"},
			},
		},
		{
			Name:       "Cloud Events/Logs",
			Status:     "Degraded Performance",
			StatusCode: 300,
			Containers: []containerResponse{
				{Name: "Prefect Cloud"},
			},
		},
	})

	if len(components) != 2 {
		t.Fatalf("len(components) = %d, want 2", len(components))
	}

	if components[1].State != status.StateDegraded {
		t.Fatalf("components[1].State = %q, want %q", components[1].State, status.StateDegraded)
	}

	if components[1].Group != "Prefect Cloud" {
		t.Fatalf("components[1].Group = %q, want Prefect Cloud", components[1].Group)
	}
}
