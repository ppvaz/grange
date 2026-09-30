package workspace

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Logger writes timestamped lines to LOG.md, echoing them in colour when
// stdout is a terminal.
type Logger struct {
	mu   sync.Mutex
	file *os.File
	tty  bool
}

const (
	Red    = "\033[0;31m"
	Green  = "\033[0;32m"
	Yellow = "\033[0;33m"
	Blue   = "\033[0;34m"
	reset  = "\033[0m"
)

func NewLogger(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{file: f, tty: IsTerminal(os.Stdout)}, nil
}

// Printf logs "HH:MM:SS [tag] message", with the tag in colour on a terminal.
func (l *Logger) Printf(color, tag, format string, args ...any) {
	stamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.file, "%s [%s] %s\n", stamp, tag, msg)
	if l.tty {
		fmt.Fprintf(os.Stdout, "%s %s[%s]%s %s\n", stamp, color, tag, reset, msg)
	}
}

// Output is where agent output goes: LOG.md plus extra (the agent's own
// log), and the terminal when there is one.
func (l *Logger) Output(extra io.Writer) io.Writer {
	writers := []io.Writer{lockedWriter{l}}
	if extra != nil {
		writers = append(writers, extra)
	}
	if l.tty {
		writers = append(writers, os.Stdout)
	}
	return io.MultiWriter(writers...)
}

type lockedWriter struct{ l *Logger }

func (w lockedWriter) Write(p []byte) (int, error) {
	w.l.mu.Lock()
	defer w.l.mu.Unlock()
	return w.l.file.Write(p)
}

func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
