package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/tuipcli/tuip/internal/status"
)

const (
	humanCardWidth            = 64
	humanIncidentLimit        = 10
	humanIncidentSummaryLimit = 280
	humanNonOperationalLimit  = 20
)

// WriteHuman renders a colored card per provider.
func WriteHuman(w io.Writer, response status.Response, details bool) error {
	cards := make([]string, 0, len(response.Results))
	for _, result := range response.Results {
		cards = append(cards, renderCard(result, details))
	}

	_, err := fmt.Fprintln(w, strings.Join(cards, "\n"))
	if err != nil {
		return fmt.Errorf("write human output: %w", err)
	}

	return nil
}

func renderCard(snapshot status.Snapshot, details bool) string {
	color := stateColor(snapshot.State)
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1).
		Width(humanCardWidth)

	title := lipgloss.NewStyle().Foreground(color).Bold(true).Render(snapshot.Name)
	stateLine := "State:   " + lipgloss.NewStyle().Foreground(color).Bold(true).Render(snapshot.State.Display())

	lines := []string{
		title,
		stateLine,
		"Summary: " + snapshot.Summary,
		"Checked: " + formatTime(snapshot.CheckedAt),
	}
	if snapshot.UpdatedAt != nil {
		lines = append(lines, "Updated: "+formatTime(*snapshot.UpdatedAt))
	}

	if snapshot.SourceURL != "" {
		lines = append(lines, "Source:  "+snapshot.SourceURL)
	}

	if snapshot.Error != "" {
		lines = append(lines, "Error:   "+snapshot.Error)
	}

	if details {
		lines = append(lines, detailLines(snapshot)...)
	}

	return border.Render(strings.Join(lines, "\n"))
}

func detailLines(snapshot status.Snapshot) []string {
	lines := []string{""}

	if len(snapshot.Incidents) == 0 {
		lines = append(lines, "Incidents: none")
	} else {
		lines = append(lines, "Incidents:")

		limit := min(len(snapshot.Incidents), humanIncidentLimit)
		for _, incident := range snapshot.Incidents[:limit] {
			label := incident.Kind
			if label == "" {
				label = "incident"
			}

			name := incident.Name
			if incident.Status != "" {
				name += " (" + incident.Status + ")"
			}

			if incident.Impact != "" {
				name += " [" + incident.Impact + "]"
			}

			lines = append(lines, fmt.Sprintf("  - %s: %s", label, name))
			if incident.Summary != "" {
				lines = append(lines, "    "+truncate(incident.Summary, humanIncidentSummaryLimit))
			}

			if incident.URL != "" {
				lines = append(lines, "    "+incident.URL)
			}
		}

		if len(snapshot.Incidents) > limit {
			lines = append(lines, fmt.Sprintf("  ... %d more", len(snapshot.Incidents)-limit))
		}
	}

	if len(snapshot.Components) == 0 {
		lines = append(lines, "Components: none exposed")

		return lines
	}

	nonOperational := make([]status.Component, 0)

	for _, component := range snapshot.Components {
		if component.State != status.StateOperational {
			nonOperational = append(nonOperational, component)
		}
	}

	if len(nonOperational) == 0 {
		lines = append(lines, fmt.Sprintf("Components: all %d operational", len(snapshot.Components)))

		return lines
	}

	lines = append(lines, fmt.Sprintf("Components: %d non-operational of %d", len(nonOperational), len(snapshot.Components)))

	limit := min(len(nonOperational), humanNonOperationalLimit)
	for _, component := range nonOperational[:limit] {
		name := component.Name
		if component.Group != "" {
			name = component.Group + " / " + name
		}

		lines = append(lines, fmt.Sprintf("  - %s: %s", name, component.Status))
	}

	if len(nonOperational) > limit {
		lines = append(lines, fmt.Sprintf("  ... %d more", len(nonOperational)-limit))
	}

	return lines
}

func stateColor(state status.State) lipgloss.Color {
	switch state {
	case status.StateOperational:
		return lipgloss.Color("42")
	case status.StateMaintenance:
		return lipgloss.Color("39")
	case status.StateDegraded:
		return lipgloss.Color("214")
	case status.StatePartialOutage:
		return lipgloss.Color("208")
	case status.StateMajorOutage:
		return lipgloss.Color("196")
	case status.StateError:
		return lipgloss.Color("201")
	case status.StateUnknown:
		fallthrough
	default:
		return lipgloss.Color("244")
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}

	return value.UTC().Format(time.RFC3339)
}

func truncate(value string, limit int) string {
	if limit <= 0 {
		return value
	}

	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	if limit <= 1 {
		return "…"
	}

	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}
