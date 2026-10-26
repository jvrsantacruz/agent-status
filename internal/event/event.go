package event

import "time"

type Event struct {
	SessionID   string `json:"session_id"`
	Harness     string `json:"harness"`
	Event       string `json:"event"`
	PID         int    `json:"pid"`
	TmuxTarget  string `json:"tmux_target"`
	TimestampMs int64  `json:"ts"`
	Message     string `json:"message"`
}

func (e *Event) Time() time.Time {
	return time.UnixMilli(e.TimestampMs)
}
