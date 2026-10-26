package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jvrsantacruz/agent-status-watcher/internal/state"
)

type StateFile struct {
	Sessions map[string]state.Session `json:"sessions"`
}

// Write atomically writes the session snapshot to path.
// Uses a temp file + rename to avoid partial reads.
func Write(path string, snapshot map[string]state.Session) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("export mkdir: %w", err)
	}

	sf := StateFile{Sessions: snapshot}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("export marshal: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("export write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("export rename: %w", err)
	}
	return nil
}
