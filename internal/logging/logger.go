package logging

import (
	"encoding/json"
	"io"
	"sync/atomic"
	"time"
)

type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

type Fields map[string]any

type Logger struct {
	entries chan map[string]any
	done    chan struct{}
	dropped atomic.Int64
}

const queueSize = 1024

func New(w io.Writer) *Logger {
	l := &Logger{
		entries: make(chan map[string]any, queueSize),
		done:    make(chan struct{}),
	}
	go l.run(w)
	return l
}

func (l *Logger) run(w io.Writer) {
	defer close(l.done)
	enc := json.NewEncoder(w)
	for entry := range l.entries {
		if err := enc.Encode(entry); err != nil {
			return
		}
	}
}

func (l *Logger) Info(event string, f Fields)  { l.write(LevelInfo, event, f) }
func (l *Logger) Warn(event string, f Fields)  { l.write(LevelWarn, event, f) }
func (l *Logger) Error(event string, f Fields) { l.write(LevelError, event, f) }

func (l *Logger) write(level Level, event string, f Fields) {
	if l == nil {
		return
	}
	entry := make(map[string]any, len(f)+3)
	for k, v := range f {
		entry[k] = v
	}
	entry["ts"] = time.Now().Format(time.RFC3339Nano)
	entry["level"] = string(level)
	entry["event"] = event

	select {
	case l.entries <- entry:
	default:
		l.dropped.Add(1)
	}
}

func (l *Logger) Dropped() int64 {
	if l == nil {
		return 0
	}
	return l.dropped.Load()
}

func (l *Logger) Close() {
	if l == nil {
		return
	}
	close(l.entries)
	<-l.done
}
