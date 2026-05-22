// Package workers tts.go applied and send text to TTS
package workers

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	rb "gotrl/internal/ringbuffer"

	gdAu "github.com/Votline/Go-audio/pkg/audio"

	"go.uber.org/zap"
)

// Tts is a tts worker
type Tts struct {
	Worker

	// acl is audio client for playing audio
	acl *gdAu.AudioClient
}

// NewTTS creates a new Tts worker
func NewTTS(call string, acl *gdAu.AudioClient, log *zap.Logger) *Tts {
	const op = "workers.NewTts"

	w := NewWorker(call, log)
	t := &Tts{Worker: w, acl: acl}

	return t
}

// TTS applies text from reader, send to TTS and play
func (t *Tts) TTS(read func([]byte) int) error {
	const op = "workers.Tts.TTS"

	defer t.log.Error("Leaved", zap.String("op", op))

	if err := EstabilishConnect(&t.Worker, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer t.conn.Close()

	textPtr := textPool.Get().(*[]byte)
	textBuf := (*textPtr)[:defaultTextLength]
	defer textPool.Put(textPtr)

	audioBuf := audioBytesPool.Get().(*rb.RingBuffer[byte])
	audioBuf.Reset()
	defer audioBytesPool.Put(audioBuf)

	errChan := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer cancel()
		defer audioBuf.Close()
		for {
			n := read(textBuf)
			if n == 0 {
				break
			}
			textFull := textBuf[:n]

			if t.mode == modeCallAPI {
				if err := t.ttsAPI(textFull, audioBuf); err != nil {
					errChan <- fmt.Errorf("%s: %w", op, err)
					break
				}
			} else {
				if err := t.ttsScript(textFull, audioBuf); err != nil {
					errChan <- fmt.Errorf("%s: %w", op, err)
					break
				}
			}
		}
	}()

	go func() {
		defer cancel()
		t.acl.Play(audioBuf)
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errChan:
		return fmt.Errorf("%s: %w", op, err)
	}
}

func (t *Tts) writeWrap(buf []byte, wr *rb.RingBuffer[byte]) int {
	dataStart := 0
	idx := bytes.Index(buf, []byte("data")) // WAV header
	if idx != -1 {
		dataStart = idx + 8
	}

	rawData := buf[dataStart:]
	if len(rawData) < 2 {
		return len(buf)
	}

	numInputSamples := len(rawData) / 2
	ratio := 1.368
	numOutputSamples := int(float64(numInputSamples) / ratio)

	if numOutputSamples <= 0 {
		return len(buf)
	}

	// Resample all buffer
	floatBufPtr := audioFloatPool.Get().(*[]float32)
	if cap(*floatBufPtr) < numOutputSamples {
		*floatBufPtr = make([]float32, numOutputSamples)
	}
	allFloats := (*floatBufPtr)[:numOutputSamples]

	for j := range numOutputSamples {
		inputIdx := int(float64(j) * ratio)
		if inputIdx*2+1 >= len(rawData) {
			allFloats = allFloats[:j]
			break
		}
		s := int16(binary.LittleEndian.Uint16(rawData[inputIdx*2 : (inputIdx+1)*2]))
		allFloats[j] = float32(s) / 32768.0
	}

	// Chunking
	samplesPerChunk := bytesPerPacket / 4
	if samplesPerChunk <= 0 {
		samplesPerChunk = len(allFloats)
	}

	for i := 0; i < len(allFloats); i += samplesPerChunk {
		end := i + samplesPerChunk
		end = min(end, len(allFloats))

		curChunk := allFloats[i:end]
		size := uint32(len(curChunk) * 4)

		if err := binary.Write(wr, binary.LittleEndian, size); err != nil {
			break
		}
		if err := binary.Write(wr, binary.LittleEndian, curChunk); err != nil {
			break
		}
	}

	audioFloatPool.Put(floatBufPtr)
	return len(buf)
}

// ttsAPI sends textFrom to API and sends audio to writer
func (t *Tts) ttsAPI(textFrom []byte, wr *rb.RingBuffer[byte]) error {
	const op = "workers.Tts.ttsAPI"

	write := func(buf []byte) int {
		return t.writeWrap(buf, wr)
	}

	if err := t.callAPI(textFrom, write, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// ttsScript sends textFrom to script and sends audio to writer
func (t *Tts) ttsScript(textFrom []byte, wr *rb.RingBuffer[byte]) error {
	const op = "workers.Tts.ttsScript"

	write := func(buf []byte) int {
		return t.writeWrap(buf, wr)
	}

	if err := t.callScript(textFrom, write, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
