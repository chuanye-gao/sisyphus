// Package memory manages agent conversation history with automatic
// token-based trimming. It provides short-term memory (the conversation
// so far) and a long-term memory interface for future extension.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/longway/sisyphus/internal/llm"
	"github.com/longway/sisyphus/pkg/config"
	"github.com/pkoukk/tiktoken-go"
)

// Memory manages the agent's conversation history.
// It is safe for concurrent use.
type Memory struct {
	mu        sync.RWMutex
	messages  []llm.Message
	maxMsg    int
	maxTokens int
	tke       *tiktoken.Tiktoken
	dataDir   string
}

// New creates a new Memory with the given configuration.
func New(cfg config.MemoryConfig, dataDir string) (*Memory, error) {
	tke, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, fmt.Errorf("memory: tiktoken: %w", err)
	}
	m := &Memory{
		messages:  make([]llm.Message, 0, cfg.MaxMessages),
		maxMsg:    cfg.MaxMessages,
		maxTokens: cfg.MaxTokens,
		tke:       tke,
		dataDir:   dataDir,
	}
	return m, nil
}

// Add appends one or more messages to the conversation history.
// It automatically trims old messages to stay within token and count limits.
func (m *Memory) Add(msgs ...llm.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msgs...)
	m.trimLocked()
}

// All returns a copy of all messages currently in memory.
func (m *Memory) All() []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]llm.Message, len(m.messages))
	copy(out, m.messages)
	return out
}

// Len returns the number of messages in memory.
func (m *Memory) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.messages)
}

// TokenCount estimates the total number of tokens in memory.
func (m *Memory) TokenCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.countTokensLocked()
}

// Clear resets the memory, removing all messages.
func (m *Memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = m.messages[:0]
}

// Save persists the current conversation to disk as JSON.
func (m *Memory) Save(sessionID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return fmt.Errorf("memory save: %w", err)
	}

	path := filepath.Join(m.dataDir, sessionID+".json")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("memory save: create %s: %w", path, err)
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(m.messages)
}

// Load restores conversation history from disk.
func (m *Memory) Load(sessionID string) error {
	path := filepath.Join(m.dataDir, sessionID+".json")
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("memory load: open %s: %w", path, err)
	}
	defer f.Close()

	var msgs []llm.Message
	if err := json.NewDecoder(f).Decode(&msgs); err != nil {
		return fmt.Errorf("memory load: decode: %w", err)
	}

	m.mu.Lock()
	m.messages = msgs
	m.trimLocked()
	m.mu.Unlock()
	return nil
}

// StoredAt returns the file path where the session is stored.
func (m *Memory) StoredAt(sessionID string) string {
	return filepath.Join(m.dataDir, sessionID+".json")
}

// trimLocked removes old messages to stay within limits. Must hold m.mu write lock.
func (m *Memory) trimLocked() {
	// Trim by count
	if m.maxMsg > 0 && len(m.messages) > m.maxMsg {
		excess := len(m.messages) - m.maxMsg
		// Keep the first system message if present
		cut := excess
		if len(m.messages) > 0 && m.messages[0].Role == "system" {
			cut = excess
		}
		if cut >= len(m.messages) {
			cut = len(m.messages)
		}
		// Remove old messages but preserve the tail
		m.messages = m.messages[cut:]
	}

	// Trim by tokens
	if m.maxTokens > 0 {
		for m.countTokensLocked() > m.maxTokens && len(m.messages) > 2 {
			start := 1 // skip system message
			if m.messages[0].Role != "system" {
				start = 0
			}
			m.messages = append(m.messages[:start], m.messages[start+1:]...)
		}
	}
}

func (m *Memory) countTokensLocked() int {
	total := 0
	for _, msg := range m.messages {
		total += len(m.tke.Encode(msg.Content, nil, nil))
	}
	return total
}

// Store is the long-term memory interface (to be implemented).
type Store interface {
	Put(entry Entry) error
	Search(query string, limit int) ([]Entry, error)
}

// Entry is a record in long-term memory.
type Entry struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
