package event

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Reader reads JSONL events from a FIFO.
type Reader struct {
	path string
}

func NewReader(path string) *Reader {
	return &Reader{path: path}
}

// Ready ensures the FIFO exists. Creates it if missing.
func (r *Reader) Ready() error {
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("event reader: create dir %s: %w", dir, err)
	}
	// mkfifo: no-op if already exists
	if err := syscall.Mkfifo(r.path, 0666); err != nil && !os.IsExist(err) {
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
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue // drop malformed lines
		}
		select {
		case out <- e:
		case <-stop:
			return io.EOF
		}
	}
	return sc.Err()
}
