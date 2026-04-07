package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Transport handles JSON-RPC 2.0 message framing over an io.ReadWriter.
// Messages use Content-Length headers as defined by the LSP specification.
type Transport struct {
	r  *bufio.Reader
	wm sync.Mutex // serialises writes
	w  io.Writer
}

// NewTransport creates a transport that reads from r and writes to w.
func NewTransport(r io.Reader, w io.Writer) *Transport {
	return &Transport{
		r: bufio.NewReaderSize(r, 64*1024),
		w: w,
	}
}

// Read reads the next JSON-RPC request from the transport.
// It blocks until a complete message is available or an error occurs.
func (t *Transport) Read() (*Request, error) {
	contentLen := -1

	// Read headers until blank line.
	for {
		line, err := t.r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		key, val, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}

		if strings.EqualFold(key, "Content-Length") {
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("parse Content-Length %q: %w", val, err)
			}
			contentLen = n
		}
		// Content-Type and other headers are ignored per spec.
	}

	if contentLen < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLen)
	if _, err := io.ReadFull(t.r, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode request: %w", err)
	}

	return &req, nil
}

// WriteResponse sends a JSON-RPC response.
func (t *Transport) WriteResponse(resp *Response) error {
	return t.writeJSON(resp)
}

func (t *Transport) writeJSON(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	t.wm.Lock()
	defer t.wm.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(t.w, header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := t.w.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}

	return nil
}
