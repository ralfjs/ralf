// Package lsp implements an LSP server for ralf.
package lsp

import "encoding/json"

// JSON-RPC 2.0 message types.

// Request is a JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // nil for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification returns true if the request has no ID (notification).
func (r *Request) IsNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// ResponseError is a JSON-RPC 2.0 error object.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC and LSP error codes used by the server.
const (
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeServerNotInit  = -32002
)

// InitializeParams is sent by the client in the initialize request.
// ProcessID and RootURI are pointers because the LSP spec allows null values.
type InitializeParams struct {
	ProcessID *int    `json:"processId"`
	RootURI   *string `json:"rootUri"`
}

// InitializeResult is the server's response to initialize.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerInfo identifies the server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ServerCapabilities declares what the server supports.
type ServerCapabilities struct {
	TextDocumentSync   *TextDocumentSyncOptions `json:"textDocumentSync,omitempty"`
	CodeActionProvider *CodeActionOptions       `json:"codeActionProvider,omitempty"`
	DefinitionProvider bool                     `json:"definitionProvider,omitempty"`
	ReferencesProvider bool                     `json:"referencesProvider,omitempty"`
	HoverProvider      bool                     `json:"hoverProvider,omitempty"`
}

// CodeActionOptions declares the code action kinds the server supports.
type CodeActionOptions struct {
	CodeActionKinds []CodeActionKind `json:"codeActionKinds,omitempty"`
}

// CodeActionKind is a hierarchical string identifying a class of code action.
type CodeActionKind string

// Supported code action kinds.
const (
	CodeActionQuickFix     CodeActionKind = "quickfix"
	CodeActionSourceFixAll CodeActionKind = "source.fixAll"
)

// TextDocumentSyncOptions describes how the server wants document changes.
type TextDocumentSyncOptions struct {
	OpenClose bool                 `json:"openClose"`
	Change    TextDocumentSyncKind `json:"change"`
	Save      *SaveOptions         `json:"save,omitempty"`
}

// TextDocumentSyncKind defines how text documents are synced.
type TextDocumentSyncKind int

// SyncFull means full content is sent on each change.
const SyncFull TextDocumentSyncKind = 1

// SaveOptions describes save notification options.
type SaveOptions struct {
	IncludeText bool `json:"includeText,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification (server → client, no ID).
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// PublishDiagnosticsParams is sent via textDocument/publishDiagnostics.
type PublishDiagnosticsParams struct {
	URI         string        `json:"uri"`
	Diagnostics []LDiagnostic `json:"diagnostics"`
}

// LDiagnostic is an LSP diagnostic (0-based line/character).
// Named LDiagnostic to avoid collision with engine.Diagnostic.
type LDiagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
	Code     string             `json:"code,omitempty"`
}

// Range is a 0-based character range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position is a 0-based line/character position in a text document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// DiagnosticSeverity maps to LSP severity levels.
type DiagnosticSeverity int

// LSP diagnostic severity constants.
const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// DidOpenTextDocumentParams is sent when a document is opened.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// TextDocumentItem represents a text document transferred from client to server.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// DidChangeTextDocumentParams is sent when a document changes.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// VersionedTextDocumentIdentifier identifies a document with a version.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentContentChangeEvent describes a content change (full sync = whole text).
type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

// DidSaveTextDocumentParams is sent when a document is saved.
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidCloseTextDocumentParams is sent when a document is closed.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// TextDocumentIdentifier identifies a text document by URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// --- Code action types ---

// CodeActionParams is sent by the client in textDocument/codeAction.
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

// CodeActionContext carries the diagnostics the code action should address.
type CodeActionContext struct {
	Diagnostics []LDiagnostic    `json:"diagnostics"`
	Only        []CodeActionKind `json:"only,omitempty"`
}

// CodeAction represents a change the editor can perform on behalf of the user.
type CodeAction struct {
	Title       string         `json:"title"`
	Kind        CodeActionKind `json:"kind,omitempty"`
	Diagnostics []LDiagnostic  `json:"diagnostics,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
}

// WorkspaceEdit represents changes to many resources managed in the workspace.
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

// TextEdit is a textual edit applicable to a text document.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}
