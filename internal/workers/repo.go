// Package workers repo.go contains structs and consts for workers
package workers

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"unsafe"

	"gotrl/internal/ringbuffer"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// modeCallAPI is flag to use API
	modeCallAPI = -1

	// modeCallScript is flag to use script
	modeCallScript = -2

	// defaultTextLength is a default text length
	// Used for pool
	defaultTextLength = 512

	// defaultAudioLength is a default audio length
	// Used for audio pool
	defaultAudioLength = 4096

	// jsonStart is a original json pattern
	// Used for start inflector data
	jsonStart = `{"original": `

	// jsonMid is a translated json patter
	// Used for middle inflector data
	jsonMid = `, "translated": `

	// jsonEnd is a end json
	// Used for end inflector data
	jsonEnd = `}`

	// sampleRate is a sample rate for audio
	sampleRate = 24000

	// channels is a number of channels for audio
	channels = 1

	// duration is a duration for collect audio
	duration = 60

	// samplePerPacket is a number of samples per packet
	samplePerPacket = ((sampleRate * duration) / 1000) * channels

	// bytesPerPacket is a number of bytes per packet
	bytesPerPacket = samplePerPacket * 2
)

// textPool is a pool for text
// Used in translator and inflector
var textPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, defaultTextLength)
		return &b
	},
}

// audioBytesPool is a pool for audio bytes (wav/websocket)
// Used in tts and stt
var audioBytesPool = sync.Pool{
	New: func() any {
		b := ringbuffer.NewRB[byte](defaultAudioLength)
		return b
	},
}

// audioFloatPool is a pool for audio floats (pcm)
// Used in tts
var audioFloatPool = sync.Pool{
	New: func() any {
		b := make([]float32, defaultAudioLength)
		return &b
	},
}

// Worker is a translator worker
type Worker struct {
	// conn is a websocket connection for call API
	conn *websocket.Conn

	// log is a zap logger
	log *zap.Logger

	// call is a url or command for call API
	call []string

	// mode is a flag to use API or script
	mode int
}

// NewWorker creates a new Worker
func NewWorker(call string, log *zap.Logger) Worker {
	w := Worker{log: log}
	w.mode = extractMode(call)
	w.call = strings.Split(call, " ")

	return w
}

// EstabilishConnect establishes websocket connection to API
// And save it to Worker 'conn' field
func EstabilishConnect(w *Worker, op string) error {
	w.log.Info("Estabilish connect",
		zap.String("op", op),
		zap.String("call", w.call[0]))

	if w.mode == modeCallAPI {
		conn, _, err := websocket.DefaultDialer.Dial(w.call[0], nil)
		if err != nil {
			return fmt.Errorf("%s: estabilished connection with %q: %w",
				op, w.call[0], err)
		}
		w.conn = conn
	}

	return nil
}

// callAPI calls API, send textFrom and read result via websocket
// Write result to writer
func (w *Worker) callAPI(textFrom []byte, write func([]byte) int, op string) error {
	w.log.Info("Call API",
		zap.String("op", op),
		zap.String("call", w.call[0]))

	if err := w.conn.WriteMessage(websocket.TextMessage, textFrom); err != nil {
		return fmt.Errorf("%s: write message: %w", op, err)
	}

	_, message, err := w.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("%s: read message: %w", op, err)
	}

	w.log.Info("Got response from API",
		zap.String("op", op),
		zap.Int("response length", len(message)))

	write(message)

	return nil
}

// callScript calls script, send textFrom to stdin and read result from stdout
// Write result to writer
// Using 'resBytes' from pool to avoid memory allocation
func (w *Worker) callScript(textFrom []byte, write func([]byte) int, op string) error {
	w.log.Info("Call script",
		zap.String("op", op),
		zap.Strings("call", w.call))

	cmd := exec.Command(w.call[0], w.call[1:]...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("%s: get stdin pipe: %w", op, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%s: get stdout pipe: %w", op, err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: start command: %w", op, err)
	}
	defer cmd.Wait()

	if _, err := stdin.Write(textFrom); err != nil {
		return fmt.Errorf("%s: stdin write: %w", op, err)
	}

	if err := stdin.Close(); err != nil {
		return fmt.Errorf("%s: close stdin: %w", op, err)
	}

	resFullPtr := textPool.Get().(*[]byte)
	resFull := (*resFullPtr)[:defaultTextLength]
	defer textPool.Put(resFullPtr)

	n, err := io.ReadFull(stdout, resFull)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return fmt.Errorf("%s: stdout read: %w", op, err)
	}
	res := resFull[:n]

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s: script wait: %w", op, err)
	}

	write(res)

	w.log.Info("Got text from script",
		zap.String("op", op),
		zap.String("text", unsafe.String(unsafe.SliceData(res), len(res))))

	return nil
}

// extractMode extracts mode from call
// It returns callAPI if call contains 'ws'
// It returns callScript if call doesn't contain 'ws'
func extractMode(call string) int {
	if strings.Contains(call, "ws") {
		return modeCallAPI
	}
	return modeCallScript
}

// trimSpaceBytes trims spaces with no allocation
func trimSpaceBytes(b *[]byte) {
	buf := *b

	if len(buf) == 0 {
		return
	}

	start := 0
	end := len(buf)

	for start < end && isSpace(buf[start]) {
		start++
	}
	for end > start && isSpace(buf[end-1]) {
		end--
	}

	if start == 0 && end == len(buf) {
		return
	} else {
		*b = buf[start:end]
	}
}

// isSpace checks if byte is space
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
