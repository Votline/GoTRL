// Package workers translator.go used Bridge-TRL/translator worker
// It applied text from user and translate it.
package workers

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"unsafe"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// callAPI is flag to use API
	callAPI = -1

	// callScript is flag to use script
	callScript = -2

	translatorDefaulLength = 512
)

var translatorTextPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, translatorDefaulLength)
		return &b
	},
}

// Translator is a translator worker
type Translator struct {
	// conn is a websocket connection for call API
	conn *websocket.Conn

	// upgrader is a websocket upgrader for call API
	upg *websocket.Upgrader

	// log is a zap logger
	log *zap.Logger

	// call is a url or command for call API
	call []string

	// mode is a flag to use API or script
	mode int
}

// NewTranslator creates a new Translator worker
// It finds 'http' in call and uses API if it is true
func NewTranslator(call string, wait int, log *zap.Logger) *Translator {
	const op = "workers.NewTranslator"

	t := &Translator{log: log}

	mode := callScript
	if strings.Contains(call, "ws") {
		mode = callAPI
		t.call = []string{call}
	} else {
		parts := strings.Split(call, " ")
		t.call = parts
	}

	upgrader := &websocket.Upgrader{}

	t.upg = upgrader
	t.mode = mode

	return t
}

// Translate translates the text from reader and writes it to writer
func (t *Translator) Translate(r io.Reader, w io.Writer) error {
	const op = "workers.Translator.Translate"

	if t.mode == callAPI {
		conn, _, err := websocket.DefaultDialer.Dial(t.call[0], nil)
		if err != nil {
			return fmt.Errorf("%s: estabilished connection with %q: %w",
				op, t.call[0], err)
		}
		t.conn = conn
		defer t.conn.Close()
	}

	textFullPtr := translatorTextPool.Get().(*[]byte)
	textFull := (*textFullPtr)[:translatorDefaulLength]
	defer translatorTextPool.Put(textFullPtr)

	for {
		n, err := r.Read(textFull)
		if err != nil {
			if err == io.EOF {
				t.log.Info("Got EOF", zap.String("op", op))
				break
			}
			return fmt.Errorf("%s: reader read: %w", op, err)
		}
		if len(textFull) == 0 {
			continue
		}

		textFrom := textFull[:n]

		t.log.Info("Got text",
			zap.String("op", op),
			zap.String("text", unsafe.String(unsafe.SliceData(textFrom), len(textFrom))))

		if t.mode == callAPI {
			if err := t.translateAPI(textFrom, w); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		} else {
			if err := t.translateScript(textFrom, w); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		}
	}

	return nil
}

// translateAPI translates the 'textFrom' using API
func (t *Translator) translateAPI(textFrom []byte, w io.Writer) error {
	const op = "workers.Translator.translateAPI"

	if err := t.conn.WriteMessage(websocket.TextMessage, textFrom); err != nil {
		return fmt.Errorf("%s: write message: %w", op, err)
	}

	_, message, err := t.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("%s: read message: %w", op, err)
	}

	t.log.Info("Got text from API",
		zap.String("op", op),
		zap.String("text", unsafe.String(unsafe.SliceData(message), len(message))))

	if _, err := w.Write(message); err != nil {
		return fmt.Errorf("%s: writer write: %w", op, err)
	}

	return nil
}

// translateScript translates the 'textFrom' using script
// Using 'resBytes' from pool to avoid memory allocation
func (t *Translator) translateScript(textFrom []byte, w io.Writer) error {
	const op = "workers.Translator.translateScript"

	cmd := exec.Command(t.call[0], t.call[1:]...)

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

	resFullPtr := translatorTextPool.Get().(*[]byte)
	resFull := (*resFullPtr)[:translatorDefaulLength]
	defer translatorTextPool.Put(resFullPtr)

	n, err := io.ReadFull(stdout, resFull)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return fmt.Errorf("%s: stdout read: %w", op, err)
	}
	res := resFull[:n]

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s: script wait: %w", op, err)
	}

	if _, err := w.Write(res); err != nil {
		return fmt.Errorf("%s: writer write: %w", op, err)
	}

	t.log.Info("Got text from script",
		zap.String("op", op),
		zap.String("text", unsafe.String(unsafe.SliceData(res), len(res))))

	return nil
}
