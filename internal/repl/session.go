package repl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionInfo describes a saved session.
type SessionInfo struct {
	ID      string
	ModTime time.Time
}

// SessionManager manages session persistence on disk.
type SessionManager struct {
	baseDir string // e.g. ~/.local/share/sisyphus
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(baseDir string) *SessionManager {
	return &SessionManager{baseDir: baseDir}
}

// SessionDir returns the directory where sessions are stored.
func (sm *SessionManager) SessionDir() string {
	return filepath.Join(sm.baseDir, "sessions")
}

// NewSessionID generates a new session ID based on the current timestamp.
func (sm *SessionManager) NewSessionID() string {
	return fmt.Sprintf("session-%s", time.Now().Format("20060102-150405"))
}

// List returns all saved sessions, sorted by modification time (newest first).
func (sm *SessionManager) List() ([]SessionInfo, error) {
	dir := sm.SessionDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("session list: %w", err)
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		sessions = append(sessions, SessionInfo{
			ID:      id,
			ModTime: info.ModTime(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	return sessions, nil
}

// Delete removes a saved session.
func (sm *SessionManager) Delete(id string) error {
	path := filepath.Join(sm.SessionDir(), id+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("session delete: %w", err)
	}
	return nil
}
