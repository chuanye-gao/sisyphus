// Package logger provides a simple leveled logger for Sisyphus.
//
// It supports four levels (DEBUG, INFO, WARN, ERROR), optional debug mode,
// and writes cleanly formatted lines with consistent prefixes.
//
// In REPL mode, logs are typically suppressed except for errors, so the
// interactive interface stays clean. Use SetOutput to redirect logs.
package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents the severity of a log message.
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// levelLabel returns a compact label for the level.
func (l Level) label() string {
	switch l {
	case DEBUG:
		return "DEBU"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERRO"
	default:
		return "????"
	}
}

// Logger is a simple leveled logger with consistent formatting.
// It is safe for concurrent use.
type Logger struct {
	mu     sync.Mutex
	prefix string // component prefix, e.g. "agent", "main"
	level  Level  // minimum level to emit
	out    io.Writer
	debug  bool // if false, DEBUG messages are suppressed regardless of level
}

// New creates a new Logger.
//
//   - prefix: a short component label, e.g. "sisyphus", "agent", "mcp"
//   - debug: if true, DEBUG messages are emitted
func New(prefix string, debug bool) *Logger {
	return &Logger{
		prefix: prefix,
		level:  DEBUG, // default: emit everything if debug is on
		out:    os.Stderr,
		debug:  debug,
	}
}

// SetOutput changes the writer. The default is os.Stderr.
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = w
}

// SetLevel sets the minimum level to emit.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetDebug enables or disables debug output.
func (l *Logger) SetDebug(d bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debug = d
}

// Debug emits a DEBUG message.
func (l *Logger) Debug(format string, args ...any) {
	l.log(DEBUG, format, args...)
}

// Info emits an INFO message.
func (l *Logger) Info(format string, args ...any) {
	l.log(INFO, format, args...)
}

// Warn emits a WARN message.
func (l *Logger) Warn(format string, args ...any) {
	l.log(WARN, format, args...)
}

// Error emits an ERROR message.
func (l *Logger) Error(format string, args ...any) {
	l.log(ERROR, format, args...)
}

// Fatal emits an ERROR message and exits with code 1.
func (l *Logger) Fatal(format string, args ...any) {
	l.log(ERROR, format, args...)
	os.Exit(1)
}

// Fatalf is an alias for Fatal to match the standard log.Fatalf signature.
func (l *Logger) Fatalf(format string, args ...any) {
	l.Fatal(format, args...)
}

// Printf emits an INFO message. It exists for compatibility with log.Printf callers.
func (l *Logger) Printf(format string, args ...any) {
	l.Info(format, args...)
}

// Println emits an INFO message.
func (l *Logger) Println(args ...any) {
	l.Info("%s", fmt.Sprint(args...))
}

func (l *Logger) log(level Level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Suppress DEBUG if debug mode is off.
	if level == DEBUG && !l.debug {
		return
	}

	// Honor level threshold.
	if level < l.level {
		return
	}

	now := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)

	// Format: HH:MM:SS.mmm [LEVEL] [prefix] message
	fmt.Fprintf(l.out, "%s [%s] [%s] %s\n", now, level.label(), l.prefix, msg)
}
