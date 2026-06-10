package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ikan31/tuip/internal/status"
)

// WriteJSON renders the normalized response for scripts/tests.
func WriteJSON(w io.Writer, response status.Response) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(response)
	if err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}

	return nil
}
