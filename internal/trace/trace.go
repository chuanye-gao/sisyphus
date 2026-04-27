package trace

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Field is one structured trace key/value pair.
type Field struct {
	Key   string
	Value any
}

func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}

// Line formats a human-readable trace event using the same shape as the
// process logger: time, level, component, event, then stable key=value fields.
func Line(level, component, event string, fields ...Field) string {
	return fmt.Sprintf("%s [%s] [%s] %s%s",
		time.Now().Format("15:04:05.000"),
		levelLabel(level),
		component,
		event,
		formatFields(fields),
	)
}

// JSONLine formats a trace event as JSONL for machines.
func JSONLine(level, component, event string, fields ...Field) string {
	m := map[string]any{
		"ts":        time.Now().Format(time.RFC3339Nano),
		"level":     strings.ToLower(level),
		"component": component,
		"event":     event,
	}
	for _, field := range fields {
		if field.Key == "" {
			continue
		}
		m[field.Key] = field.Value
	}
	data, err := json.Marshal(m)
	if err != nil {
		return Line("ERRO", component, "trace.encode_error", F("error", err.Error()))
	}
	return string(data)
}

func Truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 12 {
		return s[:max]
	}
	return s[:max-12] + "...[truncated]"
}

func levelLabel(level string) string {
	level = strings.ToUpper(level)
	switch level {
	case "DEBUG", "DEBU":
		return "DEBU"
	case "INFO":
		return "INFO"
	case "WARN":
		return "WARN"
	case "ERROR", "ERRO":
		return "ERRO"
	default:
		return "INFO"
	}
}

func formatFields(fields []Field) string {
	if len(fields) == 0 {
		return ""
	}
	copied := append([]Field(nil), fields...)
	sort.SliceStable(copied, func(i, j int) bool {
		return copied[i].Key < copied[j].Key
	})

	var b strings.Builder
	for _, field := range copied {
		if field.Key == "" {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(field.Key)
		b.WriteByte('=')
		b.WriteString(formatValue(field.Value))
	}
	return b.String()
}

func formatValue(v any) string {
	switch value := v.(type) {
	case string:
		if value == "" {
			return `""`
		}
		if needsQuote(value) {
			data, _ := json.Marshal(value)
			return string(data)
		}
		return value
	case fmt.Stringer:
		return formatValue(value.String())
	default:
		return fmt.Sprint(value)
	}
}

func needsQuote(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '"' || r == '=' {
			return true
		}
	}
	return false
}
