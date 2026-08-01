package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Outcome string

const (
	OutcomeAllowed  Outcome = "allowed"
	OutcomeBlocked  Outcome = "blocked"
	OutcomeBypassed Outcome = "bypassed"
)

type Entry struct {
	Timestamp  time.Time `json:"ts"`
	Ecosystem  string    `json:"ecosystem"`
	Package    string    `json:"package"`
	Version    string    `json:"version"`
	Outcome    Outcome   `json:"outcome"`
	Rule       string    `json:"rule"`
	Reason     string    `json:"reason"`
	ProjectDir string    `json:"project"`
}

type Log struct {
	path string
}

func New(path string) *Log {
	return &Log{path: path}
}

func (l *Log) Write(ctx context.Context, e Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	e.Timestamp = time.Now().UTC()
	return json.NewEncoder(f).Encode(e)
}
