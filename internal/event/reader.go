// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

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

// Events returns a channel of parsed events, closed when stop is closed or
// the FIFO fails.
//
// The reader holds a write end of its own FIFO open. Without it, each writer
// that closes gives the reader end of file, and until the reader reopens,
// a writer finds no reader and its event is lost.
func (r *Reader) Events(stop <-chan struct{}) <-chan Event {
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		// Non-blocking open: there is no writer yet. Reads still wait for
		// data, in Go's poller.
		f, err := os.OpenFile(r.path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return
		}
		w, err := os.OpenFile(r.path, os.O_WRONLY, 0)
		if err != nil {
			f.Close()
			return
		}
		go func() {
			<-stop
			f.Close()
			w.Close()
		}()
		_ = r.readLines(f, out, stop)
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
