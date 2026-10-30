// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package event

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaderReadyCreatesFifo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.fifo")
	rd := NewReader(path)
	err := rd.Ready()
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeNamedPipe)
}

func TestReaderReadyIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.fifo")
	rd := NewReader(path)
	require.NoError(t, rd.Ready())
	require.NoError(t, rd.Ready())
}

func TestReaderReadyCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "in.fifo")
	rd := NewReader(path)
	err := rd.Ready()
	require.NoError(t, err)
	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestReaderReceivesEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.fifo")
	rd := NewReader(path)
	require.NoError(t, rd.Ready())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := rd.Events(ctx.Done())

	writeFifo(t, path, `{"session_id":"t1","event":"working","ts":100}`+"\n")

	select {
	case e := <-ch:
		assert.Equal(t, "t1", e.SessionID)
		assert.Equal(t, "working", e.Event)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestReaderMultipleEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.fifo")
	rd := NewReader(path)
	require.NoError(t, rd.Ready())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := rd.Events(ctx.Done())

	writeFifo(t, path, `{"session_id":"t1","event":"working","ts":100}`+"\n")
	e1 := readEvent(t, ch)
	assert.Equal(t, "t1", e1.SessionID)

	writeFifo(t, path, `{"session_id":"t2","event":"idle","ts":200}`+"\n")
	e2 := readEvent(t, ch)
	assert.Equal(t, "t2", e2.SessionID)
}

func TestReaderMalformedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.fifo")
	rd := NewReader(path)
	var rejected []string
	rd.Rejected = func(line []byte, err error) { rejected = append(rejected, err.Error()) }
	require.NoError(t, rd.Ready())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := rd.Events(ctx.Done())

	writeFifo(t, path, "not json\n")
	writeFifo(t, path, `{"event":"idle","ts":100}`+"\n")
	writeFifo(t, path, `{"session_id":"t2","event":"idle","ts":200}`+"\n")

	e := readEvent(t, ch)
	assert.Equal(t, "t2", e.SessionID)
	require.Len(t, rejected, 2)
	assert.Equal(t, "no session_id", rejected[1])
}

func TestReaderReadyPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rt")
	path := filepath.Join(dir, "in.fifo")
	require.NoError(t, NewReader(path).Ready())

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o077, "fifo open to others: %v", info.Mode().Perm())
}

func TestReaderNonBlockingWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.fifo")
	rd := NewReader(path)
	require.NoError(t, rd.Ready())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := rd.Events(ctx.Done())

	writeFifo(t, path, `{"session_id":"t-nb","event":"working","ts":100}`+"\n")

	select {
	case e := <-ch:
		assert.Equal(t, "t-nb", e.SessionID)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout — non-blocking writer not picked up")
	}
}

func writeFifo(t *testing.T, path, data string) {
	t.Helper()
	// Retry open with O_NONBLOCK until the reader is ready
	var f *os.File
	var err error
	for range 50 {
		f, err = os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
		if t.Failed() {
			return
		}
	}
	require.NoError(t, err, "open fifo for write after retry")
	_, err = f.WriteString(data)
	require.NoError(t, err)
	f.Close()
}

func readEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for event")
		return Event{}
	}
}
