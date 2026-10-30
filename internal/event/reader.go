package event

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Reader reads JSONL events from a FIFO.
type Reader struct {
	path string
	// Rejected, when set, is told about every line that is not a valid event.
	Rejected func(line []byte, err error)
}

var errNoSession = errors.New("no session_id")

func NewReader(path string) *Reader {
	return &Reader{path: path}
}

// Ready ensures the FIFO exists. Creates it if missing.
func (r *Reader) Ready() error {
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("event reader: create dir %s: %w", dir, err)
	}
	// mkfifo: no-op if already exists
	if err := syscall.Mkfifo(r.path, 0o600); err != nil && !os.IsExist(err) {
		return fmt.Errorf("event reader: mkfifo %s: %w", r.path, err)
	}
	return nil
}

// Events returns a channel of parsed events. Blocks on the FIFO open;
// once open, reads lines forever. Close the FIFO or send a signal to stop.
func (r *Reader) Events(stop <-chan struct{}) <-chan Event {
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		for {
			f, err := os.OpenFile(r.path, os.O_RDONLY, 0)
			if err != nil {
				return
			}
			err = r.readLines(f, out, stop)
			f.Close()
			if err != nil {
				return
			}
			// FIFO closed on the writer side — reopen on next write
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	return out
}

func (r *Reader) readLines(f io.Reader, out chan<- Event, stop <-chan struct{}) error {
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Event
		err := json.Unmarshal(sc.Bytes(), &e)
		if err == nil && e.SessionID == "" {
			err = errNoSession
		}
		if err != nil {
			if r.Rejected != nil {
				r.Rejected(sc.Bytes(), err)
			}
			continue
		}
		select {
		case out <- e:
		case <-stop:
			return io.EOF
		}
	}
	return sc.Err()
}
