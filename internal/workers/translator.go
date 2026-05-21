// Package workers translator.go used applied text from user and translate it.
package workers

import (
	"fmt"
	"unsafe"

	"go.uber.org/zap"
)

// Translator is a translator worker
type Translator struct {
	Worker
}

// NewTranslator creates a new Translator worker
// It finds 'http' in call and uses API if it is true
func NewTranslator(call string, log *zap.Logger) *Translator {
	const op = "workers.NewTranslator"

	w := NewWorker(call, log)
	t := &Translator{Worker: w}

	return t
}

// Translate translates the text from reader and writes it to writer
func (t *Translator) Translate(read func([]byte) int, w func([]byte) int) error {
	const op = "workers.Translator.Translate"

	if err := EstabilishConnect(&t.Worker, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer t.conn.Close()

	textFullPtr := textPool.Get().(*[]byte)
	textFull := (*textFullPtr)[:defaultTextLength]
	defer textPool.Put(textFullPtr)

	for {
		n := read(textFull)
		if len(textFull) == 0 {
			continue
		}

		textFrom := textFull[:n]

		t.log.Info("Got text",
			zap.String("op", op),
			zap.String("text", unsafe.String(unsafe.SliceData(textFrom), len(textFrom))))

		if t.mode == modeCallAPI {
			if err := t.translateAPI(textFrom, w); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		} else {
			if err := t.translateScript(textFrom, w); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		}
	}
}

// translateAPI translates the 'textFrom' using API
// Used workers.repo.callAPI
func (t *Translator) translateAPI(textFrom []byte, w func([]byte) int) error {
	const op = "workers.Translator.translateAPI"

	if err := t.callAPI(textFrom, w, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// translateScript translates the 'textFrom' using script
// Using 'resBytes' from pool to avoid memory allocation
// Used workers.repo.callScript
func (t *Translator) translateScript(textFrom []byte, w func([]byte) int) error {
	const op = "workers.Translator.translateScript"

	if err := t.callScript(textFrom, w, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
