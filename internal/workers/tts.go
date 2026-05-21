// Package workers tts.go applied and send text to TTS
package workers

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	rb "gotrl/internal/ringbuffer"

	gd "github.com/Votline/Go-audio"
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
func NewTTS(call string, log *zap.Logger) *Tts {
	const op = "workers.NewTts"

	w := NewWorker(call, log)

	logF := func(msg string) {
		log.Info("AudioClient", zap.String("event", msg))
	}

	acl, err := gd.InitAudioClient(
		0, 0, 0, 0,
		channels, 0, sampleRate, duration,
		false, logF,
	)
	if err != nil {
		log.Fatal("InitAudioClient", zap.Error(err))
	}

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

// writeWrap uploads audio to buffer via chunks
func writeWrap(buf []byte, wr *rb.RingBuffer[byte]) int {
	idx := bytes.Index(buf, []byte("data")) // WAV header
	if idx == -1 {
		return 0
	}
	dataStart := idx + 8 // len 'data' + chunk size
	if dataStart >= len(buf) {
		return 0
	}

	for i := dataStart; i < len(buf); i += bytesPerPacket {
		end := i + bytesPerPacket
		end = min(end, len(buf))

		curChunk := buf[i:end]
		numSamples := len(curChunk) / 2

		floatBufPtr := audioFloatPool.Get().(*[]float32)
		floatBuf := (*floatBufPtr)[:numSamples]

		for j := range numSamples {
			s := int16(binary.LittleEndian.Uint16(curChunk[j*2 : (j+1)*2]))
			floatBuf[j] = float32(s) / 32768
		}

		size := uint32(len(floatBuf) * 4)
		if err := binary.Write(wr, binary.LittleEndian, size); err != nil {
			audioFloatPool.Put(floatBufPtr)
			return 0
		}
		if err := binary.Write(wr, binary.LittleEndian, floatBuf); err != nil {
			audioFloatPool.Put(floatBufPtr)
			return 0
		}
		audioFloatPool.Put(floatBufPtr)
	}

	return len(buf)
}

// ttsAPI sends textFrom to API and sends audio to writer
func (t *Tts) ttsAPI(textFrom []byte, wr *rb.RingBuffer[byte]) error {
	const op = "workers.Tts.ttsAPI"

	write := func(buf []byte) int {
		return writeWrap(buf, wr)
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
		return writeWrap(buf, wr)
	}

	if err := t.callScript(textFrom, write, op); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
