package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/crossfile"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/parser"
	"github.com/ralfjs/ralf/internal/project"
	"github.com/ralfjs/ralf/internal/version"
)

// Server lifecycle states.
const (
	stateEmpty        = 0 // before initialize
	stateInitializing = 1 // after initialize response, before initialized
	stateReady        = 2 // after initialized notification
	stateShutdown     = 3 // after shutdown request
)

// lintDebounce is the delay before processing queued didChange lint requests.
const lintDebounce = 100 * time.Millisecond

// cachedLint stores the last lint result for a file, used by code actions.
type cachedLint struct {
	engineDiags []engine.Diagnostic
	lspDiags    []LDiagnostic
	source      []byte
	lineStarts  []int
}

// Server is the ralf LSP server. It owns the lint engine and project state,
// and communicates with the editor over a JSON-RPC 2.0 transport.
type Server struct {
	eng   *engine.Engine
	cfg   *config.Config
	graph *project.Graph // nil when no cross-file rules are active

	transport *Transport
	state     int // stateEmpty → stateInitialized → stateShutdown
	exit      int // -1 while running; 0 or 1 once exit received

	ctx     context.Context // Run context; cancelled on shutdown/disconnect
	docs    *docStore
	lintReq chan string   // file paths needing debounced lint
	done    chan struct{} // closed when lintLoop exits

	cacheMu   sync.Mutex
	lintCache map[string]*cachedLint // path → last lint result
}

// NewServer creates an LSP server with the given engine and config.
// The graph parameter may be nil when no cross-file rules are active.
func NewServer(eng *engine.Engine, cfg *config.Config, graph *project.Graph) *Server {
	return &Server{
		eng:       eng,
		cfg:       cfg,
		graph:     graph,
		exit:      -1,
		docs:      newDocStore(),
		lintReq:   make(chan string, 64),
		done:      make(chan struct{}),
		lintCache: make(map[string]*cachedLint),
	}
}

// Run starts the main loop, reading JSON-RPC messages from r and writing
// responses to w. It blocks until the client sends exit, the transport closes,
// or the context is cancelled.
func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	s.transport = NewTransport(r, w)
	s.ctx = ctx

	go s.lintLoop(ctx)
	defer func() {
		close(s.lintReq)
		<-s.done
	}()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		req, err := s.transport.Read()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) {
				slog.Debug("transport closed")
				return nil
			}
			slog.Error("transport read", "error", err)
			return fmt.Errorf("transport read: %w", err)
		}

		if s.dispatch(req) {
			return nil
		}
	}
}

// dispatch handles a single request. Returns true if the server should exit.
func (s *Server) dispatch(req *Request) bool {
	// After shutdown, only exit is allowed per LSP spec.
	if s.state == stateShutdown {
		if req.Method == "exit" {
			s.handleExit()
			return true
		}
		s.sendError(req, CodeInvalidRequest, "server is shutting down")
		return false
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		// Ignore if we haven't completed initialize.
		if s.state != stateInitializing {
			return false
		}
		s.handleInitialized()
	case "shutdown":
		// Shutdown requires prior initialization.
		if s.state == stateEmpty {
			s.sendError(req, CodeServerNotInit, "server not initialized")
			return false
		}
		s.handleShutdown(req)
	case "exit":
		s.handleExit()
		return true
	case "textDocument/didOpen":
		if s.state != stateReady {
			return false
		}
		s.handleDidOpen(req)
	case "textDocument/didChange":
		if s.state != stateReady {
			return false
		}
		s.handleDidChange(req)
	case "textDocument/didSave":
		if s.state != stateReady {
			return false
		}
		s.handleDidSave(req)
	case "textDocument/didClose":
		if s.state != stateReady {
			return false
		}
		s.handleDidClose(req)
	case "textDocument/codeAction":
		if s.state != stateReady {
			if !req.IsNotification() {
				s.sendError(req, CodeServerNotInit, "server not initialized")
			}
			return false
		}
		s.handleCodeAction(req)
	default:
		if s.state != stateReady {
			// Silently ignore notifications before ready.
			if req.IsNotification() {
				return false
			}
			s.sendError(req, CodeServerNotInit, "server not initialized")
			return false
		}
		if !req.IsNotification() {
			s.sendError(req, CodeMethodNotFound, fmt.Sprintf("method not found: %s", req.Method))
		}
	}
	return false
}

func (s *Server) handleInitialize(req *Request) {
	if s.state != stateEmpty {
		s.sendError(req, CodeInvalidRequest, "server already initialized")
		return
	}

	var params InitializeParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req, CodeInvalidParams, "invalid initialize params")
			return
		}
	}

	var rootURI string
	if params.RootURI != nil {
		rootURI = *params.RootURI
	}

	slog.Debug("initialize", "rootURI", rootURI, "processID", params.ProcessID)

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: &TextDocumentSyncOptions{
				OpenClose: true,
				Change:    SyncFull,
				Save:      &SaveOptions{IncludeText: false},
			},
			CodeActionProvider: &CodeActionOptions{
				CodeActionKinds: []CodeActionKind{CodeActionQuickFix, CodeActionSourceFixAll},
			},
			DefinitionProvider: true,
			ReferencesProvider: true,
			HoverProvider:      true,
		},
		ServerInfo: &ServerInfo{
			Name:    "ralf",
			Version: version.Version,
		},
	}

	s.state = stateInitializing
	s.sendResult(req, result)
}

func (s *Server) handleInitialized() {
	s.state = stateReady
	slog.Debug("initialized")
}

func (s *Server) handleShutdown(req *Request) {
	slog.Debug("shutdown")
	s.state = stateShutdown
	s.sendResult(req, nil)
}

func (s *Server) handleExit() {
	if s.state == stateShutdown {
		s.exit = 0
	} else {
		s.exit = 1
	}
	slog.Debug("exit", "code", s.exit)
}

// ExitCode returns the exit code set by the exit notification.
// 0 = clean (shutdown called first), 1 = dirty (exit without shutdown),
// -1 = no exit received.
func (s *Server) ExitCode() int {
	return s.exit
}

func (s *Server) sendResult(req *Request, result any) {
	if req.IsNotification() {
		return
	}

	resp := &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
	if err := s.transport.WriteResponse(resp); err != nil {
		slog.Error("write response", "method", req.Method, "error", err)
	}
}

func (s *Server) sendError(req *Request, code int, message string) {
	if req.IsNotification() {
		return
	}

	resp := &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error:   &ResponseError{Code: code, Message: message},
	}
	if err := s.transport.WriteResponse(resp); err != nil {
		slog.Error("write error response", "method", req.Method, "error", err)
	}
}

// --- Document sync handlers ---

func (s *Server) handleDidOpen(req *Request) {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		slog.Error("didOpen: invalid params", "error", err)
		return
	}

	path := URIToPath(params.TextDocument.URI)
	s.docs.Open(path, []byte(params.TextDocument.Text))

	slog.Debug("didOpen", "path", path)
	s.lintAndPublish(s.ctx, path)
}

func (s *Server) handleDidChange(req *Request) {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		slog.Error("didChange: invalid params", "error", err)
		return
	}

	if len(params.ContentChanges) == 0 {
		return
	}

	path := URIToPath(params.TextDocument.URI)
	content := []byte(params.ContentChanges[len(params.ContentChanges)-1].Text)
	s.docs.Update(path, content)

	select {
	case s.lintReq <- path:
	default:
		slog.Debug("lint queue full, dropping", "path", path)
	}
}

func (s *Server) handleDidSave(req *Request) {
	var params DidSaveTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		slog.Error("didSave: invalid params", "error", err)
		return
	}

	path := URIToPath(params.TextDocument.URI)
	slog.Debug("didSave", "path", path)
	s.lintAndPublish(s.ctx, path)
}

func (s *Server) handleDidClose(req *Request) {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		slog.Error("didClose: invalid params", "error", err)
		return
	}

	path := URIToPath(params.TextDocument.URI)
	s.docs.Close(path)

	s.cacheMu.Lock()
	delete(s.lintCache, path)
	s.cacheMu.Unlock()

	slog.Debug("didClose", "path", path)
	s.publishDiagnostics(PathToURI(path), []LDiagnostic{})
}

// --- Lint loop and diagnostics ---

// lintLoop runs in a background goroutine, debouncing didChange lint requests.
func (s *Server) lintLoop(ctx context.Context) {
	defer close(s.done)

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	pending := make(map[string]struct{})

	for {
		select {
		case path, ok := <-s.lintReq:
			if !ok {
				for p := range pending {
					s.lintAndPublish(ctx, p)
				}
				return
			}
			pending[path] = struct{}{}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(lintDebounce)

		case <-timer.C:
			for p := range pending {
				s.lintAndPublish(ctx, p)
			}
			pending = make(map[string]struct{})

		case <-ctx.Done():
			return
		}
	}
}

// lintAndPublish lints a single file and publishes diagnostics to the client.
func (s *Server) lintAndPublish(ctx context.Context, path string) {
	if _, ok := parser.LangFromPath(path); !ok {
		return
	}

	content, ok := s.docs.Get(path)
	if !ok {
		// Document not open — skip to avoid stale publishes after didClose.
		return
	}

	engineDiags := s.eng.LintFile(ctx, path, content)

	if s.graph != nil && crossfile.HasActiveRules(s.cfg) {
		imports, exports, err := project.ExtractFile(ctx, path, content)
		if err == nil {
			update := s.graph.UpdateFile(path, exports, imports)
			if update.GraphChanged {
				crossDiags := crossfile.Run(s.graph, s.cfg)

				// Group cross-file diagnostics by file.
				crossByFile := make(map[string][]engine.Diagnostic)
				for _, cd := range crossDiags {
					crossByFile[cd.File] = append(crossByFile[cd.File], cd)
				}
				engineDiags = append(engineDiags, crossByFile[path]...)

				// Publish updated diagnostics for other affected open files.
				for otherPath, otherCross := range crossByFile {
					if otherPath == path {
						continue
					}
					otherContent, open := s.docs.Get(otherPath)
					if !open {
						continue
					}
					otherDiags := s.eng.LintFile(ctx, otherPath, otherContent)
					otherDiags = append(otherDiags, otherCross...)
					s.publishDiagnostics(PathToURI(otherPath), convertDiagnostics(otherDiags, otherContent))
				}
			}
		} else {
			slog.Debug("extract failed, skipping graph update", "path", path, "error", err)
		}
	}

	uri := PathToURI(path)
	lineStarts := buildLineIndex(content)
	lspDiags := convertDiagnosticsWithIndex(engineDiags, content, lineStarts)

	s.cacheMu.Lock()
	s.lintCache[path] = &cachedLint{
		engineDiags: engineDiags,
		lspDiags:    lspDiags,
		source:      content,
		lineStarts:  lineStarts,
	}
	s.cacheMu.Unlock()

	s.publishDiagnostics(uri, lspDiags)
}

func (s *Server) publishDiagnostics(uri string, diags []LDiagnostic) {
	n := &Notification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params: PublishDiagnosticsParams{
			URI:         uri,
			Diagnostics: diags,
		},
	}
	if err := s.transport.WriteNotification(n); err != nil {
		slog.Error("publish diagnostics", "uri", uri, "error", err)
	}
}
