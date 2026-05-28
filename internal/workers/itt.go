// Package workers itt.go capture image, send to itt and write result
// Use maim to capture image
package workers

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/gorilla/websocket"

	"go.uber.org/zap"
)

// ITT is a itt worker
type ITT struct {
	Worker
}

// NewITT creates a new itt worker
func NewITT(call string, log *zap.Logger) *ITT {
	const op = "workers.Newitt"

	w := NewWorker(call, log)

	s := &ITT{Worker: w}

	return s
}

// ITT applies text from reader, send to itt and play
func (i *ITT) ITT(write func([]byte) int) error {
	const op = "workers.itt.itt"

	defer i.log.Error("Leaved", zap.String("op", op))

	if err := EstabilishConnect(&i.Worker, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer i.conn.Close()

	errChan := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer cancel()
		for {
			_, msg, err := i.conn.ReadMessage()
			if err != nil {
				errChan <- fmt.Errorf("%s: ReadMessage: %w", op, err)
				break
			}
			write(msg)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			cmd := exec.Command("maim", "-s")

			data, err := cmd.Output()
			if err != nil {
				i.log.Error("ReadFile", zap.Error(err))
				continue
			}

			if err := i.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				i.log.Error("WriteMessage", zap.Error(err))
				continue
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errChan:
		return fmt.Errorf("%s: %w", op, err)
	}
}
