package obs

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

type Logger struct {
	w           io.Writer
	requestUUID string
	mu          sync.Mutex
}

func New(w io.Writer, requestUUID string) *Logger {
	if w == nil {
		w = os.Stderr
	}
	return &Logger{w: w, requestUUID: requestUUID}
}

func (l *Logger) Info(event string, fields map[string]any) {
	l.emit("info", event, fields)
}

func (l *Logger) Error(event string, fields map[string]any) {
	l.emit("error", event, fields)
}

func (l *Logger) emit(level, event string, fields map[string]any) {
	line := map[string]any{
		"ts":           time.Now().UTC().Format(time.RFC3339Nano),
		"level":        level,
		"request_uuid": l.requestUUID,
		"event":        event,
	}
	for k, v := range fields {
		if v == nil {
			continue
		}
		if _, reserved := line[k]; reserved {
			continue
		}
		line[k] = v
	}

	buf, err := json.Marshal(line)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(buf)
	_, _ = l.w.Write([]byte("\n"))
}
