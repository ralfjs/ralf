package lsp

import (
	"context"
	"encoding/json"
	"io"
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

// initialize performs the initialize/initialized handshake.
func (h *testHarness) initialize(t *testing.T) {
	t.Helper()

	resp := h.request(t, 1, "initialize", InitializeParams{
		ProcessID: 1,
		RootURI:   "file:///tmp/project",
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
		ProcessID: 42,
		RootURI:   "file:///tmp/project",
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

	h.notify(t, "exit")
	h.wait(t)
}

func TestServer_DoubleInitialize(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.initialize(t)

	// Second initialize should fail.
	resp := h.request(t, 2, "initialize", InitializeParams{
		RootURI: "file:///tmp/project",
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
