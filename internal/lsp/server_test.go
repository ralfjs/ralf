package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

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

	srv := NewServer(eng, cfg)
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
