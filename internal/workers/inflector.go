// Package workers inflector.go applied text, collect it and inflect
// It split text by periods or wait 1 second and inflect it.
package workers

import (
	"bytes"
	"fmt"
	"io"
	"unsafe"

	"go.uber.org/zap"
)

// Inflector is a inflector worker
type Inflector struct {
	Worker
}

// NewInflector creates a new Inflector worker
func NewInflector(call string, log *zap.Logger) *Inflector {
	const op = "workers.NewInflector"

	w := NewWorker(call, log)
	t := &Inflector{Worker: w}

	return t
}

// Inflect inflects the text from reader and writes it to writer
func (i *Inflector) Inflect(r io.Reader, w io.Writer) error {
	const op = "workers.Inflector.Inflect"

	if err := EstabilishConnect(&i.Worker); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer i.conn.Close()

	textPartPtr := translatorTextPool.Get().(*[]byte)
	textPart := (*textPartPtr)[:defaultTextLength]
	defer translatorTextPool.Put(textPartPtr)

	textFullPtr := translatorTextPool.Get().(*[]byte)
	textFull := (*textFullPtr)[:0]
	defer translatorTextPool.Put(textFullPtr)

	for {
		n, err := r.Read(textPart)
		if err != nil {
			if err == io.EOF {
				i.log.Info("Got EOF", zap.String("op", op))
				break
			}
			return fmt.Errorf("%s: reader read: %w", op, err)
		}
		if len(textPart) == 0 {
			continue
		}

		textFrom := textPart[:n]
		if !bytes.ContainsAny(textFrom, ".?!:;") {
			textFull = append(textFull, textFrom...)
			i.log.Info("Got text",
				zap.String("op", op),
				zap.String("text", unsafe.String(unsafe.SliceData(textFrom), len(textFrom))))
			continue
		}
		textFull = append(textFull, textFrom...)

		i.log.Info("Got text",
			zap.String("op", op),
			zap.String("text", unsafe.String(unsafe.SliceData(textFull), len(textFull))))

		if i.mode == modeCallAPI {
			if err := i.inflectAPI(textFull, w); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		} else {
			if err := i.inflectScript(textFull, w); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		}

		// reset textFull
		textFull = textFull[:0]
	}

	return nil
}

// inflectAPI inflects the 'textFrom' using API
// Used workers.repo.callAPI
func (i *Inflector) inflectAPI(textFrom []byte, w io.Writer) error {
	const op = "workers.Inflector.inflectAPI"

	if err := i.callAPI(textFrom, w); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// inflectScript inflects the 'textFrom' using script
// Using 'resBytes' from pool to avoid memory allocation
// Used workers.repo.callScript
func (i *Inflector) inflectScript(textFrom []byte, w io.Writer) error {
	const op = "workers.Inflector.inflectScript"

	if err := i.callScript(textFrom, w); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
