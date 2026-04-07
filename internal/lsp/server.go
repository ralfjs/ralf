package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/version"
)

// Server is the ralf LSP server. It owns the lint engine and project state,
// and communicates with the editor over a JSON-RPC 2.0 transport.
type Server struct {
	eng *engine.Engine
	cfg *config.Config

	transport   *Transport
	initialized bool
	shutdown    bool
	exit        int // -1 while running; 0 or 1 once exit received
}

// NewServer creates an LSP server with the given engine and config.
func NewServer(eng *engine.Engine, cfg *config.Config) *Server {
	return &Server{
		eng:  eng,
		cfg:  cfg,
		exit: -1,
	}
}

// Run starts the main loop, reading JSON-RPC messages from r and writing
// responses to w. It blocks until the client sends exit, the transport closes,
// or the context is cancelled.
func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	s.transport = NewTransport(r, w)

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
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		s.handleInitialized()
	case "shutdown":
		s.handleShutdown(req)
	case "exit":
		s.handleExit()
		return true
	default:
		if s.shutdown {
			s.sendError(req, CodeInvalidRequest, "server is shutting down")
			return false
		}
		if !s.initialized {
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
	if s.initialized {
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

	slog.Debug("initialize", "rootURI", params.RootURI, "processID", params.ProcessID)

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: &TextDocumentSyncOptions{
				OpenClose: true,
				Change:    SyncFull,
				Save:      &SaveOptions{IncludeText: false},
			},
			CodeActionProvider: true,
			DefinitionProvider: true,
			ReferencesProvider: true,
			HoverProvider:      true,
		},
		ServerInfo: &ServerInfo{
			Name:    "ralf",
			Version: version.Version,
		},
	}

	s.sendResult(req, result)
}

func (s *Server) handleInitialized() {
	s.initialized = true
	slog.Debug("initialized")
}

func (s *Server) handleShutdown(req *Request) {
	slog.Debug("shutdown")
	s.shutdown = true
	s.sendResult(req, nil)
}

func (s *Server) handleExit() {
	if s.shutdown {
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
		slog.Warn("error on notification", "method", req.Method, "message", message)
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
