package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"testing"
)

// buildMessage builds a raw Content-Length framed JSON-RPC message.
func buildMessage(method string, id int) []byte {
	body, _ := json.Marshal(Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(fmt.Sprintf("%d", id)),
		Method:  method,
	})
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
}

func BenchmarkTransport_Read(b *testing.B) {
	msg := buildMessage("textDocument/didOpen", 1)

	// Pre-build a buffer with N copies of the message.
	var buf bytes.Buffer
	for range b.N {
		buf.Write(msg)
	}

	tr := NewTransport(&buf, io.Discard)

	b.ResetTimer()
	b.SetBytes(int64(len(msg)))

	for range b.N {
		if _, err := tr.Read(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransport_WriteResponse(b *testing.B) {
	tr := NewTransport(bytes.NewReader(nil), io.Discard)

	resp := &Response{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Result:  map[string]string{"status": "ok"},
	}

	b.ResetTimer()

	for range b.N {
		if err := tr.WriteResponse(resp); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransport_Roundtrip(b *testing.B) {
	// Simulate a full request/response cycle over pipes.
	// A single long-lived server goroutine reads requests and responds.
	srvR, clientW := io.Pipe()
	clientR, srvW := io.Pipe()
	defer func() {
		_ = clientW.Close()
		_ = srvR.Close()
		_ = clientR.Close()
		_ = srvW.Close()
	}()

	client := NewTransport(clientR, clientW)
	server := NewTransport(srvR, srvW)

	resp := &Response{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Result:  map[string]string{"status": "ok"},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			req, err := server.Read()
			if err != nil {
				return
			}
			r := *resp
			r.ID = req.ID
			if err := server.WriteResponse(&r); err != nil {
				return
			}
		}
	}()

	b.ResetTimer()

	for range b.N {
		_ = client.writeJSON(Request{
			JSONRPC: "2.0",
			ID:      json.RawMessage("1"),
			Method:  "test",
		})

		// Read response from client side (raw framing).
		contentLen := -1
		for {
			line, err := client.r.ReadString('\n')
			if err != nil {
				b.Fatal(err)
			}
			line = stripCRLF(line)
			if line == "" {
				break
			}
			if key, val, ok := cutHeader(line); ok && key == "Content-Length" {
				n := 0
				for _, c := range val {
					n = n*10 + int(c-'0')
				}
				contentLen = n
			}
		}
		body := make([]byte, contentLen)
		if _, err := io.ReadFull(client.r, body); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	_ = clientW.Close() // unblocks server goroutine
	<-done
}
