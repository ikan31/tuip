package output

import (
	"encoding/json"
	"io"

	"github.com/tuipcli/tuip/internal/status"
)

// WriteJSON renders the normalized response for scripts/tests.
func WriteJSON(w io.Writer, response status.Response) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(response)
}
