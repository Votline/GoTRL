// Package workers stt.go record audio, send to STT and write to writer
package workers

import (
	"context"
	"fmt"

	gdAu "github.com/Votline/Go-audio/pkg/audio"
	"github.com/gorilla/websocket"

	"go.uber.org/zap"
)

// Stt is a Stt worker
type Stt struct {
	Worker

	// acl is audio client for record audio
	acl *gdAu.AudioClient
}

// NewStt creates a new Stt worker
func NewStt(call string, acl *gdAu.AudioClient, log *zap.Logger) *Stt {
	const op = "workers.NewStt"

	w := NewWorker(call, log)

	s := &Stt{Worker: w, acl: acl}

	return s
}

// Stt applies text from reader, send to Stt and play
func (s *Stt) Stt(write func([]byte) int) error {
	const op = "workers.Stt.Stt"

	defer s.log.Error("Leaved", zap.String("op", op))

	if err := EstabilishConnect(&s.Worker, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer s.conn.Close()

	errChan := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer cancel()
		for {
			_, msg, err := s.conn.ReadMessage()
			if err != nil {
				errChan <- fmt.Errorf("%s: ReadMessage: %w", op, err)
				break
			}
			write(msg)
		}
	}()

	go func() {
		defer cancel()
		if err := s.acl.Record(s); err != nil {
			errChan <- fmt.Errorf("%s: start record: %w", op, err)
			return
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errChan:
		return fmt.Errorf("%s: %w", op, err)
	}
}

// Write writes p to websocket connection
func (s *Stt) Write(p []byte) (int, error) {
	if len(p) <= 4 {
		return len(p), nil
	}

	err := s.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, fmt.Errorf("write message: %w", err)
	}
	return len(p), nil
}
