// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package event

import (
	"bufio"
	"bytes"
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

// Events returns a channel of parsed events, closed when stop is closed or
// the FIFO fails.
//
// The FIFO is opened read-write, so the daemon holds a write end of its own
// FIFO. Without it, each writer that closes gives the reader end of file,
// and until the reader reopens, a writer finds no reader and its event is
// lost. Read-write also keeps the read blocking, which Darwin needs: Go
// cannot poll a FIFO there (golang.org/issue/24164).
func (r *Reader) Events(stop <-chan struct{}) <-chan Event {
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		f, err := os.OpenFile(r.path, os.O_RDWR, 0)
		if err != nil {
			return
		}
		defer f.Close()
		go func() {
			<-stop
			// Wake a read blocked on an empty FIFO; readLines then sees stop.
			_, _ = f.Write([]byte("\n"))
		}()
		_ = r.readLines(f, out, stop)
	}()
	return out
}

func (r *Reader) readLines(f io.Reader, out chan<- Event, stop <-chan struct{}) error {
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		select {
		case <-stop:
			return io.EOF
		default:
		}
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
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
