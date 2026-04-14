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
	CodeActionProvider bool                     `json:"codeActionProvider,omitempty"`
	DefinitionProvider bool                     `json:"definitionProvider,omitempty"`
	ReferencesProvider bool                     `json:"referencesProvider,omitempty"`
	HoverProvider      bool                     `json:"hoverProvider,omitempty"`
}

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
