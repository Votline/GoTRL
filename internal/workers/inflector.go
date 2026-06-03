// Package workers inflector.go applied text, collect it and inflect
// It split text by periods or wait 1 second and inflect it.
package workers

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"gotrl/internal/utils"

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

	if err := EstablishConnect(&i.Worker, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer i.conn.Close()

	textPartPtr := textPool.Get().(*[]byte)
	textPart := (*textPartPtr)[:defaultTextLength]
	defer textPool.Put(textPartPtr)

	textFullPtr := textPool.Get().(*[]byte)
	textFull := (*textFullPtr)[:0]
	defer textPool.Put(textFullPtr)

	origFullPtr := textPool.Get().(*[]byte)
	origFull := (*origFullPtr)[:0]
	defer textPool.Put(origFullPtr)

	origPartPtr := textPool.Get().(*[]byte)
	origPart := (*origPartPtr)[:defaultTextLength]
	defer textPool.Put(origPartPtr)

	jsonReqPtr := textPool.Get().(*[]byte)
	jsonReq := (*jsonReqPtr)[:0]
	defer textPool.Put(jsonReqPtr)

	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			if len(textFull) > 0 {
				i.doInflect(&origFull, &textFull, &jsonReq, w)
			}
			mu.Unlock()
		}
	})

	wg.Go(func() {
		for {
			n := trRead(textPart)
			if n == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			textFrom := textPart[:n]
			utils.TrimSpaceBytes(&textFrom)

			nOrig := origRead(origPart)
			if nOrig > 0 {
				origTemp := origPart[:nOrig]
				utils.TrimSpaceBytes(&origTemp)
				origFull = append(origFull, origTemp...)
			}

			textFull = append(textFull, textFrom...)

			if bytes.ContainsAny(textFrom, ".?!:;") {
				i.log.Info("Got full text",
					zap.String("op", op),
					zap.Int("text", len(textFull)),
					zap.Int("orig", len(origFull)))

				mu.Lock()
				i.doInflect(&origFull, &textFull, &jsonReq, w)
				mu.Unlock()
			}
		}
	})

	wg.Wait()
	return nil
}

// doInflect sends inflection request to API
// Writes result to writer and clears buffers
func (i *Inflector) doInflect(origFull, textFull, jsonReq *[]byte, w func([]byte) int) error {
	const op = "workers.Inflector.doInflect"
	if len(*textFull) == 0 {
		return nil
	}

	utils.TrimSpaceBytes(textFull)
	utils.TrimSpaceBytes(origFull)

	requestMarshal(*origFull, *textFull, jsonReq)

	var err error
	if i.mode == modeCallAPI {
		err = i.inflectAPI(*jsonReq, w)
	} else {
		err = i.inflectScript(*jsonReq, w)
	}

	*textFull = (*textFull)[:0]
	*origFull = (*origFull)[:0]
	*jsonReq = (*jsonReq)[:0]

	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// inflectAPI inflects the 'textFrom' using API
// Used workers.repo.callAPI
func (i *Inflector) inflectAPI(textFrom []byte, w func([]byte) int) error {
	const op = "workers.Inflector.inflectAPI"

	fmt.Println(string(textFrom))

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
