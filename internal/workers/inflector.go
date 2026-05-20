// Package workers inflector.go applied text, collect it and inflect
// It split text by periods or wait 1 second and inflect it.
package workers

import (
	"bytes"
	"fmt"
	"strconv"
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
func (i *Inflector) Inflect(origRead, trRead, w func([]byte) int) error {
	const op = "workers.Inflector.Inflect"

	if err := EstabilishConnect(&i.Worker, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer i.conn.Close()

	textPartPtr := translatorTextPool.Get().(*[]byte)
	textPart := (*textPartPtr)[:defaultTextLength]
	defer translatorTextPool.Put(textPartPtr)

	textFullPtr := translatorTextPool.Get().(*[]byte)
	textFull := (*textFullPtr)[:0]
	defer translatorTextPool.Put(textFullPtr)

	origFullPtr := translatorTextPool.Get().(*[]byte)
	origFull := (*origFullPtr)[:0]
	defer translatorTextPool.Put(origFullPtr)

	jsonReqPtr := translatorTextPool.Get().(*[]byte)
	jsonReq := (*jsonReqPtr)[:0]
	defer translatorTextPool.Put(jsonReqPtr)

	for {
		n := trRead(textPart)
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
		origFull = append(origFull, textFull...)

		i.log.Info("Got text",
			zap.String("op", op),
			zap.String("text", unsafe.String(unsafe.SliceData(textFull), len(textFull))),
			zap.String("orig", unsafe.String(unsafe.SliceData(origFull), len(origFull))))

		requestMarshal(origFull, textFull, &jsonReq)

		i.log.Warn("Request",
			zap.String("op", op),
			zap.String("text", unsafe.String(unsafe.SliceData(jsonReq), len(jsonReq))))

		if i.mode == modeCallAPI {
			if err := i.inflectAPI(jsonReq, w); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		} else {
			if err := i.inflectScript(jsonReq, w); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		}

		// reset
		textFull = textFull[:0]
		jsonReq = jsonReq[:0]
		origFull = origFull[:0]
	}
}

// inflectAPI inflects the 'textFrom' using API
// Used workers.repo.callAPI
func (i *Inflector) inflectAPI(textFrom []byte, w func([]byte) int) error {
	const op = "workers.Inflector.inflectAPI"

	if err := i.callAPI(textFrom, w, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// inflectScript inflects the 'textFrom' using script
// Using 'resBytes' from pool to avoid memory allocation
// Used workers.repo.callScript
func (i *Inflector) inflectScript(textFrom []byte, w func([]byte) int) error {
	const op = "workers.Inflector.inflectScript"

	if err := i.callScript(textFrom, w, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// requestMarshal marshals the 'orig' and 'trln' to json
func requestMarshal(orig, trln []byte, buf *[]byte) {
	jsonData := (*buf)[:0]

	trimSpaceBytes(&orig)
	trimSpaceBytes(&trln)

	origStr := unsafe.String(unsafe.SliceData(orig), len(orig))
	trlnStr := unsafe.String(unsafe.SliceData(trln), len(trln))

	jsonData = append(jsonData, jsonStart...)
	jsonData = strconv.AppendQuote(jsonData, origStr)
	jsonData = append(jsonData, jsonMid...)
	jsonData = strconv.AppendQuote(jsonData, trlnStr)
	jsonData = append(jsonData, jsonEnd...)

	*buf = jsonData
}
