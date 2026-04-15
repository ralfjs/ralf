package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/version"
)

// testHarness wires up a Server and a client Transport over in-memory pipes.
type testHarness struct {
	srv    *Server
	client *Transport
	done   chan error
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "Use let or const instead of var",
			},
		},
	}
	eng, errs := engine.New(cfg)
	if len(errs) > 0 {
		t.Fatalf("engine init: %v", errs)
	}

	// Two pipes: server reads from srvR, writes to srvW.
	// Client reads from clientR, writes to clientW.
	srvR, clientW := io.Pipe()
	clientR, srvW := io.Pipe()
	t.Cleanup(func() {
		_ = srvR.Close()
		_ = clientW.Close()
		_ = clientR.Close()
		_ = srvW.Close()
	})

	srv := NewServer(eng, cfg, nil)
	h := &testHarness{
		srv:    srv,
		client: NewTransport(clientR, clientW),
		done:   make(chan error, 1),
	}

	go func() {
		h.done <- srv.Run(context.Background(), srvR, srvW)
	}()

	return h
}

// request sends a JSON-RPC request and reads back the response.
func (h *testHarness) request(t *testing.T, id int, method string, params any) *Response {
	t.Helper()
	h.send(t, id, method, params)
	return h.readResp(t)
}

// send sends a JSON-RPC request without reading the response.
func (h *testHarness) send(t *testing.T, id int, method string, params any) {
	t.Helper()

	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		rawParams = b
	}

	idBytes, _ := json.Marshal(id)
	req := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		ID:      idBytes,
		Method:  method,
		Params:  rawParams,
	}

	if err := h.client.writeJSON(req); err != nil {
		t.Fatalf("send request %q: %v", method, err)
	}
}

// notify sends a JSON-RPC notification (no ID).
func (h *testHarness) notify(t *testing.T, method string) {
	t.Helper()

	msg := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
	}{
		JSONRPC: "2.0",
		Method:  method,
	}

	if err := h.client.writeJSON(msg); err != nil {
		t.Fatalf("send notification %q: %v", method, err)
	}
}

// notifyWithParams sends a JSON-RPC notification with params.
func (h *testHarness) notifyWithParams(t *testing.T, method string, params any) {
	t.Helper()

	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		rawParams = b
	}

	msg := struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  rawParams,
	}

	if err := h.client.writeJSON(msg); err != nil {
		t.Fatalf("send notification %q: %v", method, err)
	}
}

// message is a generic JSON-RPC message (response or notification).
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// tryReadMessage reads the next JSON-RPC message without calling t.Fatal,
// making it safe to call from a non-test goroutine.
func (h *testHarness) tryReadMessage() (message, error) {
	contentLen := -1
	for {
		line, err := h.client.r.ReadString('\n')
		if err != nil {
			return message{}, fmt.Errorf("read message header: %w", err)
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

	if contentLen < 0 {
		return message{}, fmt.Errorf("missing Content-Length in message")
	}

	body := make([]byte, contentLen)
	if _, err := io.ReadFull(h.client.r, body); err != nil {
		return message{}, fmt.Errorf("read message body: %w", err)
	}

	var msg message
	if err := json.Unmarshal(body, &msg); err != nil {
		return message{}, fmt.Errorf("decode message: %w (body: %s)", err, body)
	}

	return msg, nil
}

// readMessage reads the next JSON-RPC message (response or notification).
func (h *testHarness) readMessage(t *testing.T) message {
	t.Helper()
	msg, err := h.tryReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

// readMessageTimeout reads the next message with a timeout. The read runs in
// a separate goroutine using tryReadMessage (no t.Fatal from non-test goroutine).
func (h *testHarness) readMessageTimeout(t *testing.T, d time.Duration) message {
	t.Helper()
	type result struct {
		msg message
		err error
	}
	ch := make(chan result, 1)
	go func() {
		msg, err := h.tryReadMessage()
		ch <- result{msg, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("readMessageTimeout: %v", r.err)
		}
		return r.msg
	case <-time.After(d):
		t.Fatal("timed out waiting for message")
		return message{}
	}
}

// readResp reads a JSON-RPC response from the client side.
func (h *testHarness) readResp(t *testing.T) *Response {
	t.Helper()

	contentLen := -1
	for {
		line, err := h.client.r.ReadString('\n')
		if err != nil {
			t.Fatalf("read response header: %v", err)
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

	if contentLen < 0 {
		t.Fatal("missing Content-Length in response")
	}

	body := make([]byte, contentLen)
	if _, err := io.ReadFull(h.client.r, body); err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, body)
	}

	return &resp
}

func intPtr(n int) *int       { return &n }
func strPtr(s string) *string { return &s }

// initialize performs the initialize/initialized handshake.
func (h *testHarness) initialize(t *testing.T) {
	t.Helper()

	resp := h.request(t, 1, "initialize", InitializeParams{
		ProcessID: intPtr(1),
		RootURI:   strPtr("file:///tmp/project"),
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %s", resp.Error.Message)
	}
	h.notify(t, "initialized")
}

// wait waits for the server Run loop to finish.
func (h *testHarness) wait(t *testing.T) {
	t.Helper()
	if err := <-h.done; err != nil {
		t.Fatalf("server Run error: %v", err)
	}
}

func stripCRLF(s string) string {
	for s != "" && (s[len(s)-1] == '\r' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

func cutHeader(s string) (key, val string, ok bool) {
	for i := range len(s) - 1 {
		if s[i] == ':' && s[i+1] == ' ' {
			return s[:i], s[i+2:], true
		}
	}
	return "", "", false
}

func TestServer_InitializeShutdownExit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// Initialize.
	resp := h.request(t, 1, "initialize", InitializeParams{
		ProcessID: intPtr(42),
		RootURI:   strPtr("file:///tmp/project"),
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %s", resp.Error.Message)
	}

	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if result.ServerInfo == nil || result.ServerInfo.Name != "ralf" {
		t.Fatalf("expected server name 'ralf', got %v", result.ServerInfo)
	}
	if result.ServerInfo.Version != version.Version {
		t.Fatalf("expected version %s, got %s", version.Version, result.ServerInfo.Version)
	}
	if result.Capabilities.TextDocumentSync == nil {
		t.Fatal("expected text document sync capabilities")
	}
	if result.Capabilities.TextDocumentSync.Change != SyncFull {
		t.Fatalf("expected SyncFull, got %d", result.Capabilities.TextDocumentSync.Change)
	}
	if !result.Capabilities.CodeActionProvider {
		t.Fatal("expected CodeActionProvider")
	}
	if !result.Capabilities.DefinitionProvider {
		t.Fatal("expected DefinitionProvider")
	}
	if !result.Capabilities.ReferencesProvider {
		t.Fatal("expected ReferencesProvider")
	}
	if !result.Capabilities.HoverProvider {
		t.Fatal("expected HoverProvider")
	}

	// Initialized notification.
	h.notify(t, "initialized")

	// Shutdown.
	resp = h.request(t, 2, "shutdown", nil)
	if resp.Error != nil {
		t.Fatalf("shutdown error: %s", resp.Error.Message)
	}

	// Exit.
	h.notify(t, "exit")
	h.wait(t)

	if h.srv.ExitCode() != 0 {
		t.Fatalf("expected exit code 0, got %d", h.srv.ExitCode())
	}
}

func TestServer_InitializeWithNullFields(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// Initialize with null processId and rootUri (spec allows this).
	resp := h.request(t, 1, "initialize", InitializeParams{})
	if resp.Error != nil {
		t.Fatalf("initialize error: %s", resp.Error.Message)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_ExitWithoutShutdown(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.initialize(t)

	// Exit without shutdown.
	h.notify(t, "exit")
	h.wait(t)

	if h.srv.ExitCode() != 1 {
		t.Fatalf("expected exit code 1 (no shutdown), got %d", h.srv.ExitCode())
	}
}

func TestServer_RequestBeforeInit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	resp := h.request(t, 1, "textDocument/hover", nil)
	if resp.Error == nil {
		t.Fatal("expected error for request before initialize")
	}
	if resp.Error.Code != CodeServerNotInit {
		t.Fatalf("expected code %d, got %d", CodeServerNotInit, resp.Error.Code)
	}

	// Clean up.
	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_MethodNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.initialize(t)

	resp := h.request(t, 2, "textDocument/unknown", nil)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != CodeMethodNotFound {
		t.Fatalf("expected code %d, got %d", CodeMethodNotFound, resp.Error.Code)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_RejectsAfterShutdown(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.initialize(t)

	// Shutdown.
	resp := h.request(t, 2, "shutdown", nil)
	if resp.Error != nil {
		t.Fatalf("shutdown error: %s", resp.Error.Message)
	}

	// Request after shutdown should be rejected.
	resp = h.request(t, 3, "textDocument/hover", nil)
	if resp.Error == nil {
		t.Fatal("expected error for request after shutdown")
	}
	if resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("expected code %d, got %d", CodeInvalidRequest, resp.Error.Code)
	}

	// Even initialize should be rejected after shutdown.
	resp = h.request(t, 4, "initialize", nil)
	if resp.Error == nil {
		t.Fatal("expected error for initialize after shutdown")
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_DoubleInitialize(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.initialize(t)

	// Second initialize should fail.
	resp := h.request(t, 2, "initialize", InitializeParams{
		RootURI: strPtr("file:///tmp/project"),
	})
	if resp.Error == nil {
		t.Fatal("expected error for double initialize")
	}
	if resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("expected code %d, got %d", CodeInvalidRequest, resp.Error.Code)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_NotificationBeforeInit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// Unknown notification before init — should be silently ignored (no response).
	h.notify(t, "textDocument/didOpen")

	// Server should still work — send a real request to verify.
	resp := h.request(t, 1, "textDocument/hover", nil)
	if resp.Error == nil || resp.Error.Code != CodeServerNotInit {
		t.Fatal("expected ServerNotInit error")
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_ShutdownBeforeInit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// Shutdown without initialize should be rejected.
	resp := h.request(t, 1, "shutdown", nil)
	if resp.Error == nil {
		t.Fatal("expected error for shutdown before init")
	}
	if resp.Error.Code != CodeServerNotInit {
		t.Fatalf("expected code %d, got %d", CodeServerNotInit, resp.Error.Code)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_InitializedBeforeInit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// Sending initialized before initialize should be silently ignored.
	h.notify(t, "initialized")

	// Server should still accept initialize normally.
	resp := h.request(t, 1, "initialize", InitializeParams{
		ProcessID: intPtr(1),
		RootURI:   strPtr("file:///tmp/project"),
	})
	if resp.Error != nil {
		t.Fatalf("initialize after spurious initialized should work: %s", resp.Error.Message)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestTransport_Read_NegativeContentLength(t *testing.T) {
	t.Parallel()

	input := "Content-Length: -5\r\n\r\n"
	tr := NewTransport(strings.NewReader(input), io.Discard)

	_, err := tr.Read()
	if err == nil || !strings.Contains(err.Error(), "invalid Content-Length") {
		t.Fatalf("expected invalid Content-Length error, got: %v", err)
	}
}

func TestTransport_Read_ZeroContentLength(t *testing.T) {
	t.Parallel()

	input := "Content-Length: 0\r\n\r\n"
	tr := NewTransport(strings.NewReader(input), io.Discard)

	_, err := tr.Read()
	if err == nil || !strings.Contains(err.Error(), "invalid Content-Length") {
		t.Fatalf("expected invalid Content-Length error, got: %v", err)
	}
}

func TestTransport_Read_MissingContentLength(t *testing.T) {
	t.Parallel()

	input := "Bad-Header: foo\r\n\r\n"
	tr := NewTransport(strings.NewReader(input), io.Discard)

	_, err := tr.Read()
	if err == nil || !strings.Contains(err.Error(), "missing Content-Length") {
		t.Fatalf("expected missing Content-Length error, got: %v", err)
	}
}

func TestTransport_Read_InvalidContentLength(t *testing.T) {
	t.Parallel()

	input := "Content-Length: abc\r\n\r\n"
	tr := NewTransport(strings.NewReader(input), io.Discard)

	_, err := tr.Read()
	if err == nil || !strings.Contains(err.Error(), "parse Content-Length") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

func TestTransport_Read_OversizedContentLength(t *testing.T) {
	t.Parallel()

	input := "Content-Length: 999999999\r\n\r\n"
	tr := NewTransport(strings.NewReader(input), io.Discard)

	_, err := tr.Read()
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("expected oversized error, got: %v", err)
	}
}

func TestTransport_Read_TruncatedBody(t *testing.T) {
	t.Parallel()

	input := "Content-Length: 100\r\n\r\nshort"
	tr := NewTransport(strings.NewReader(input), io.Discard)

	_, err := tr.Read()
	if err == nil || !strings.Contains(err.Error(), "read body") {
		t.Fatalf("expected read body error, got: %v", err)
	}
}

func TestTransport_Read_InvalidJSON(t *testing.T) {
	t.Parallel()

	input := "Content-Length: 8\r\n\r\nnot json"
	tr := NewTransport(strings.NewReader(input), io.Discard)

	_, err := tr.Read()
	if err == nil || !strings.Contains(err.Error(), "decode request") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestTransport_Read_HeaderEOF(t *testing.T) {
	t.Parallel()

	// EOF in the middle of headers.
	tr := NewTransport(strings.NewReader("Content-Le"), io.Discard)

	_, err := tr.Read()
	if err == nil || !strings.Contains(err.Error(), "read header") {
		t.Fatalf("expected header read error, got: %v", err)
	}
}

func TestTransport_WriteNotification(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tr := NewTransport(bytes.NewReader(nil), &buf)

	err := tr.WriteNotification(&Notification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params: PublishDiagnosticsParams{
			URI:         "file:///tmp/test.js",
			Diagnostics: []LDiagnostic{},
		},
	})
	if err != nil {
		t.Fatalf("WriteNotification: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Content-Length:") {
		t.Fatal("expected Content-Length header in notification")
	}
	if !strings.Contains(out, "publishDiagnostics") {
		t.Fatal("expected publishDiagnostics method in notification body")
	}
}

func TestTransport_WriteNotification_WriterError(t *testing.T) {
	t.Parallel()

	tr := NewTransport(bytes.NewReader(nil), &failWriter{})

	err := tr.WriteNotification(&Notification{
		JSONRPC: "2.0",
		Method:  "test",
	})
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestTransport_WriteResponse_WriterError(t *testing.T) {
	t.Parallel()

	tr := NewTransport(bytes.NewReader(nil), &failWriter{})

	err := tr.WriteResponse(&Response{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Result:  "ok",
	})
	if err == nil {
		t.Fatal("expected write error")
	}
}

type failWriter struct{}

func (w *failWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestURIToPath_Basic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		uri  string
		want string
	}{
		{"file:///tmp/project", "/tmp/project"},
		{"file:///path/with%20spaces/file.ts", "/path/with spaces/file.ts"},
		{"file:///a/b/c.js", "/a/b/c.js"},
		{"/fallback/path", "/fallback/path"},
		// Windows drive letter.
		{"file:///C:/Users/test/file.js", "C:/Users/test/file.js"},
		// UNC path.
		{"file://server/share/file.js", "//server/share/file.js"},
	}

	for _, tt := range tests {
		got := URIToPath(tt.uri)
		if got != tt.want {
			t.Errorf("URIToPath(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestRequest_IsNotification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   json.RawMessage
		want bool
	}{
		{"nil id", nil, true},
		{"null id", json.RawMessage("null"), true},
		{"number id", json.RawMessage("1"), false},
		{"string id", json.RawMessage(`"abc"`), false},
	}

	for _, tt := range tests {
		r := &Request{ID: tt.id}
		if got := r.IsNotification(); got != tt.want {
			t.Errorf("IsNotification(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestServer_DidOpen_PublishesDiagnostics(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.initialize(t)

	h.notifyWithParams(t, "textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///tmp/test.js",
			LanguageID: "javascript",
			Version:    1,
			Text:       "var x = 1;\n",
		},
	})

	msg := h.readMessage(t)
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics, got method %q", msg.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if params.URI != "file:///tmp/test.js" {
		t.Fatalf("expected URI file:///tmp/test.js, got %s", params.URI)
	}
	if len(params.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic for var usage")
	}

	found := false
	for _, d := range params.Diagnostics {
		if d.Code == "no-var" {
			found = true
			if d.Range.Start.Line != 0 {
				t.Fatalf("expected line 0, got %d", d.Range.Start.Line)
			}
			if d.Severity != SeverityError {
				t.Fatalf("expected severity error (1), got %d", d.Severity)
			}
			if d.Source != "ralf" {
				t.Fatalf("expected source 'ralf', got %q", d.Source)
			}
		}
	}
	if !found {
		t.Fatal("no-var diagnostic not found")
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_DidOpen_CleanFile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.initialize(t)

	h.notifyWithParams(t, "textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///tmp/clean.js",
			LanguageID: "javascript",
			Version:    1,
			Text:       "const x = 1;\n",
		},
	})

	msg := h.readMessage(t)
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics, got method %q", msg.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if len(params.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics for clean file, got %d", len(params.Diagnostics))
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_DidClose_ClearsDiagnostics(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.initialize(t)

	// Open a file with an error.
	h.notifyWithParams(t, "textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///tmp/close.js",
			LanguageID: "javascript",
			Version:    1,
			Text:       "var x = 1;\n",
		},
	})

	// Read the diagnostics from open.
	msg := h.readMessage(t)
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics on open, got %q", msg.Method)
	}

	// Close the file.
	h.notifyWithParams(t, "textDocument/didClose", DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///tmp/close.js",
		},
	})

	// Should get empty diagnostics.
	msg = h.readMessage(t)
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics on close, got %q", msg.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if len(params.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics after close, got %d", len(params.Diagnostics))
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_DidChange_Debounced(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.initialize(t)

	// Open a clean file first.
	h.notifyWithParams(t, "textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///tmp/debounce.js",
			LanguageID: "javascript",
			Version:    1,
			Text:       "const x = 1;\n",
		},
	})

	// Read diagnostics from open.
	msg := h.readMessage(t)
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics on open, got %q", msg.Method)
	}

	// Send multiple rapid changes.
	for i := range 5 {
		h.notifyWithParams(t, "textDocument/didChange", DidChangeTextDocumentParams{
			TextDocument: VersionedTextDocumentIdentifier{
				URI:     "file:///tmp/debounce.js",
				Version: i + 2,
			},
			ContentChanges: []TextDocumentContentChangeEvent{
				{Text: "var x = " + string(rune('1'+i)) + ";\n"},
			},
		})
	}

	// Wait for debounced diagnostics with a timeout.
	msg = h.readMessageTimeout(t, 2*time.Second)
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics after debounce, got %q", msg.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if len(params.Diagnostics) == 0 {
		t.Fatal("expected diagnostics for var usage after debounced change")
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_DidSave_Relints(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.initialize(t)

	// Open with an error.
	h.notifyWithParams(t, "textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///tmp/save.js",
			LanguageID: "javascript",
			Version:    1,
			Text:       "var x = 1;\n",
		},
	})

	// Read diagnostics from open (should have errors).
	msg := h.readMessage(t)
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics on open, got %q", msg.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.Diagnostics) == 0 {
		t.Fatal("expected diagnostics on open")
	}

	// Update doc store directly to clean content, then save. Use FromSlash so
	// the key matches the server's URIToPath result on Windows too.
	h.srv.docs.Update(filepath.FromSlash("/tmp/save.js"), []byte("const x = 1;\n"))

	h.notifyWithParams(t, "textDocument/didSave", DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///tmp/save.js",
		},
	})

	// Read diagnostics from save (should be clean).
	msg = h.readMessage(t)
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics on save, got %q", msg.Method)
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics after fix, got %d", len(params.Diagnostics))
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_DidOpen_NonJSFile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.initialize(t)

	// Open a non-JS file — should not produce any diagnostics notification.
	h.notifyWithParams(t, "textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///tmp/readme.md",
			LanguageID: "markdown",
			Version:    1,
			Text:       "var x = 1;\n",
		},
	})

	// Send a request to verify the server is still responsive
	// (no notification was sent for the .md file).
	resp := h.request(t, 2, "textDocument/hover", nil)
	if resp.Error == nil || resp.Error.Code != CodeMethodNotFound {
		t.Fatalf("expected MethodNotFound, got %v", resp.Error)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestDocStore_OpenGetClose(t *testing.T) {
	t.Parallel()

	ds := newDocStore()

	// Get on empty store returns false.
	_, ok := ds.Get("/tmp/file.js")
	if ok {
		t.Fatal("expected false for missing doc")
	}

	// Open and get.
	ds.Open("/tmp/file.js", []byte("hello"))
	content, ok := ds.Get("/tmp/file.js")
	if !ok {
		t.Fatal("expected true after Open")
	}
	if string(content) != "hello" {
		t.Fatalf("expected 'hello', got %q", content)
	}

	// Get returns a copy (mutating it doesn't affect the store).
	content[0] = 'X'
	content2, _ := ds.Get("/tmp/file.js")
	if string(content2) != "hello" {
		t.Fatal("Get should return a copy, but store was mutated")
	}

	// Close and verify gone.
	ds.Close("/tmp/file.js")
	_, ok = ds.Get("/tmp/file.js")
	if ok {
		t.Fatal("expected false after Close")
	}
}

func TestDocStore_Update(t *testing.T) {
	t.Parallel()

	ds := newDocStore()

	// Update on non-existent doc is a no-op.
	ds.Update("/tmp/missing.js", []byte("new"))
	_, ok := ds.Get("/tmp/missing.js")
	if ok {
		t.Fatal("update on missing doc should not create it")
	}

	// Open, then update.
	ds.Open("/tmp/file.js", []byte("old"))
	ds.Update("/tmp/file.js", []byte("new"))

	content, ok := ds.Get("/tmp/file.js")
	if !ok {
		t.Fatal("expected doc to exist after update")
	}
	if string(content) != "new" {
		t.Fatalf("expected 'new', got %q", content)
	}
}

func TestPathToURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want string
	}{
		{"/tmp/file.js", "file:///tmp/file.js"},
		{"/a/b/c.ts", "file:///a/b/c.ts"},
		{"/path/with spaces/file.ts", "file:///path/with%20spaces/file.ts"},
		{"/path/with#hash/file.js", "file:///path/with%23hash/file.js"},
		// Windows drive letter (forward-slash form, works on any OS).
		{"C:/Users/test/file.js", "file:///C:/Users/test/file.js"},
		// UNC path (forward-slash form).
		{"//server/share/file.js", "file://server/share/file.js"},
	}

	for _, tt := range tests {
		got := PathToURI(tt.path)
		if got != tt.want {
			t.Errorf("PathToURI(%q) = %q, want %q", tt.path, got, tt.want)
		}
		// Round-trip: PathToURI → URIToPath should recover the original path.
		// Use filepath.FromSlash so the comparison works on Windows where
		// URIToPath returns backslash-separated paths.
		want := filepath.FromSlash(tt.path)
		if roundTrip := URIToPath(got); roundTrip != want {
			t.Errorf("round-trip failed: URIToPath(PathToURI(%q)) = %q, want %q", tt.path, roundTrip, want)
		}
	}
}
