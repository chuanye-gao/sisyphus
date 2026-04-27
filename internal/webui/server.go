// Package webui provides a small local browser UI for Sisyphus.
package webui

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/longway/sisyphus/internal/agent"
	"github.com/longway/sisyphus/internal/llm"
	"github.com/longway/sisyphus/internal/memory"
	"github.com/longway/sisyphus/internal/repl"
	"github.com/longway/sisyphus/internal/tool"
	"github.com/longway/sisyphus/pkg/config"
)

//go:embed logo.png
var assetFS embed.FS

// Server serves the local browser UI and bridges browser requests to Agent.Step.
type Server struct {
	agent    *agent.Agent
	registry *tool.Registry
	cfg      *config.Config
	sessions *repl.SessionManager
	debug    bool

	turnMu    sync.Mutex
	stateMu   sync.Mutex
	sessionID string
	cancel    context.CancelFunc
}

// Config describes the dependencies needed to construct a Server.
type Config struct {
	Provider  llm.Provider
	Registry  *tool.Registry
	Cfg       *config.Config
	SessionID string
	Debug     bool
}

// New creates a web Server with one shared interactive agent session.
func New(sc Config) (*Server, error) {
	dataDir := config.DataDir()
	sm := repl.NewSessionManager(dataDir)

	mem, err := memory.New(sc.Cfg.Memory, sm.SessionDir())
	if err != nil {
		return nil, fmt.Errorf("web: create memory: %w", err)
	}

	ag := agent.New(sc.Provider, mem, sc.Registry, sc.Cfg.Agent.MaxSteps, sc.Debug)
	sessionID := sc.SessionID
	if sessionID != "" {
		if err := mem.Load(sessionID); err != nil {
			return nil, fmt.Errorf("web: load session %s: %w", sessionID, err)
		}
	} else {
		sessionID = sm.NewSessionID()
		ag.InitMemory()
	}

	return &Server{
		agent:     ag,
		registry:  sc.Registry,
		cfg:       sc.Cfg,
		sessions:  sm,
		debug:     sc.Debug,
		sessionID: sessionID,
	}, nil
}

// Handler returns the HTTP handler for the web UI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /assets/logo.png", s.handleLogo)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/turn", s.handleTurn)
	mux.HandleFunc("POST /api/cancel", s.handleCancel)
	mux.HandleFunc("POST /api/clear", s.handleClear)
	mux.HandleFunc("POST /api/save", s.handleSave)
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("POST /api/sessions/load", s.handleLoadSession)
	return mux
}

func (s *Server) handleLogo(w http.ResponseWriter, r *http.Request) {
	data, err := assetFS.ReadFile("logo.png")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeContent(w, r, "logo.png", time.Time{}, bytes.NewReader(data))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = indexTemplate.Execute(w, map[string]any{
		"Model":     s.cfg.LLM.Model,
		"SessionID": s.currentSessionID(),
		"Tools":     strings.Join(s.registry.List(), ", "),
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, stateResponse{
		Model:        s.cfg.LLM.Model,
		SessionID:    s.currentSessionID(),
		Tools:        s.registry.List(),
		Busy:         s.busy(),
		Memory:       s.memoryStats(),
		ChatMessages: chatMessagesFromLLM(s.agent.Memory().All()),
	})
}

func (s *Server) handleTurn(w http.ResponseWriter, r *http.Request) {
	if !s.turnMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "another turn is already running"})
		return
	}
	defer s.turnMu.Unlock()

	var req turnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return
	}
	req.Input = strings.TrimSpace(req.Input)
	if req.Input == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx, cancel := context.WithCancel(r.Context())
	s.setCancel(cancel)
	defer func() {
		cancel()
		s.clearCancel()
	}()

	handler := newSSEHandler(w, flusher, s.debug)
	handler.send("session", map[string]any{"session_id": s.currentSessionID()})

	err := s.agent.Step(ctx, req.Input, handler)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			handler.send("cancelled", map[string]string{"message": "turn cancelled"})
		} else {
			handler.send("error", map[string]string{"message": err.Error()})
		}
		return
	}

	if saveErr := s.agent.Memory().Save(s.currentSessionID()); saveErr != nil {
		handler.send("error", map[string]string{"message": fmt.Sprintf("auto-save failed: %v", saveErr)})
		return
	}

	handler.send("done", map[string]any{
		"session_id": s.currentSessionID(),
		"memory":     s.memoryStats(),
	})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	s.stateMu.Lock()
	cancel := s.cancel
	s.stateMu.Unlock()

	if cancel == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "idle"})
		return
	}
	cancel()
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	if !s.turnMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot clear while a turn is running"})
		return
	}
	defer s.turnMu.Unlock()

	s.agent.Memory().Clear()
	s.agent.InitMemory()
	s.stateMu.Lock()
	s.sessionID = s.sessions.NewSessionID()
	sessionID := s.sessionID
	s.stateMu.Unlock()

	if err := s.agent.Memory().Save(sessionID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, stateResponse{
		Model:        s.cfg.LLM.Model,
		SessionID:    sessionID,
		Tools:        s.registry.List(),
		Busy:         false,
		Memory:       s.memoryStats(),
		ChatMessages: []chatMessage{},
	})
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	var req saveRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = s.currentSessionID()
	}
	if !validSessionID(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	if err := s.agent.Memory().Save(name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.setSessionID(name)
	writeJSON(w, http.StatusOK, map[string]string{"session_id": name})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.sessions.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]sessionSummary, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, s.sessionSummary(session))
	}
	writeJSON(w, http.StatusOK, sessionsResponse{Sessions: out})
}

func (s *Server) handleLoadSession(w http.ResponseWriter, r *http.Request) {
	if !s.turnMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot load while a turn is running"})
		return
	}
	defer s.turnMu.Unlock()

	var req loadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
		return
	}
	if !validSessionID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	if err := s.agent.Memory().Load(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.setSessionID(id)
	writeJSON(w, http.StatusOK, stateResponse{
		Model:        s.cfg.LLM.Model,
		SessionID:    id,
		Tools:        s.registry.List(),
		Busy:         false,
		Memory:       s.memoryStats(),
		ChatMessages: chatMessagesFromLLM(s.agent.Memory().All()),
	})
}

func (s *Server) currentSessionID() string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.sessionID
}

func (s *Server) setSessionID(id string) {
	s.stateMu.Lock()
	s.sessionID = id
	s.stateMu.Unlock()
}

func (s *Server) setCancel(cancel context.CancelFunc) {
	s.stateMu.Lock()
	s.cancel = cancel
	s.stateMu.Unlock()
}

func (s *Server) clearCancel() {
	s.stateMu.Lock()
	s.cancel = nil
	s.stateMu.Unlock()
}

func (s *Server) busy() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.cancel != nil
}

func (s *Server) memoryStats() memoryStats {
	return memoryStats{
		Messages: s.agent.Memory().Len(),
		Tokens:   s.agent.Memory().TokenCount(),
	}
}

func (s *Server) sessionSummary(info repl.SessionInfo) sessionSummary {
	summary := sessionSummary{
		ID:      info.ID,
		ModTime: info.ModTime.Format(time.RFC3339),
		Current: info.ID == s.currentSessionID(),
		Title:   "New chat",
	}

	msgs, err := s.readSessionMessages(info.ID)
	if err != nil {
		summary.Title = info.ID
		return summary
	}
	chat := chatMessagesFromLLM(msgs)
	summary.MessageCount = len(chat)
	for _, msg := range chat {
		if msg.Role == "user" {
			summary.Title = truncateText(oneLine(msg.Content), 52)
			break
		}
	}
	return summary
}

func (s *Server) readSessionMessages(id string) ([]llm.Message, error) {
	if !validSessionID(id) {
		return nil, fmt.Errorf("invalid session id")
	}
	path := filepath.Join(s.sessions.SessionDir(), id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var msgs []llm.Message
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type turnRequest struct {
	Input string `json:"input"`
}

type saveRequest struct {
	Name string `json:"name"`
}

type loadRequest struct {
	ID string `json:"id"`
}

type stateResponse struct {
	Model        string        `json:"model"`
	SessionID    string        `json:"session_id"`
	Tools        []string      `json:"tools"`
	Busy         bool          `json:"busy"`
	Memory       memoryStats   `json:"memory"`
	ChatMessages []chatMessage `json:"chat_messages"`
}

type memoryStats struct {
	Messages int `json:"messages"`
	Tokens   int `json:"tokens"`
}

type sessionsResponse struct {
	Sessions []sessionSummary `json:"sessions"`
}

type sessionSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ModTime      string `json:"mod_time"`
	MessageCount int    `json:"message_count"`
	Current      bool   `json:"current"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func chatMessagesFromLLM(msgs []llm.Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs))
	for _, msg := range msgs {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		switch msg.Role {
		case "user", "assistant":
			out = append(out, chatMessage{
				Role:    msg.Role,
				Content: content,
			})
		}
	}
	return out
}

func validSessionID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || id == "." || id == ".." {
		return false
	}
	if strings.Contains(id, "/") || strings.Contains(id, `\`) {
		return false
	}
	return filepath.Base(id) == id
}

func truncateText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

var indexTemplate = template.Must(template.New("index").Parse(indexHTML))
