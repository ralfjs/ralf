package parser

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Tree wraps a tree-sitter syntax tree.
//
// Every Tree must be closed after use to release C resources.
type Tree struct {
	inner *tree_sitter.Tree
}

// RootNode returns the root node of the syntax tree.
func (t *Tree) RootNode() Node {
	return newNode(t.inner.RootNode())
}

// Edit records an edit to the tree so that it can be incrementally reparsed.
func (t *Tree) Edit(edit *tree_sitter.InputEdit) {
	t.inner.Edit(edit)
}

// Close releases the underlying C resources. Safe to call multiple times.
func (t *Tree) Close() {
	if t != nil && t.inner != nil {
		t.inner.Close()
		t.inner = nil
	}
}

// Point represents a row/column position in source code (zero-based).
type Point struct {
	Row    uint
	Column uint
}

// Node is a lightweight value type wrapping a tree-sitter node.
//
// Wrapping isolates the rest of the codebase from tree-sitter internals,
// enabling future migration (e.g. to typescript-go).
type Node struct {
	inner *tree_sitter.Node
}

func newNode(n *tree_sitter.Node) Node {
	return Node{inner: n}
}

// IsNull returns true if the underlying node pointer is nil.
func (n Node) IsNull() bool {
	return n.inner == nil
}

// Kind returns the node's type name (e.g. "function_declaration").
// Note: crosses CGo and allocates a Go string via C.GoString.
// For hot loops, prefer KindID + pre-resolved symbol IDs.
func (n Node) Kind() string {
	return n.inner.Kind()
}

// KindID returns the node's type as a numeric symbol ID.
// This is a cheap CGo call with no allocation — O(1).
func (n Node) KindID() uint16 {
	return n.inner.KindId()
}

// SymbolForKind resolves a kind string to its numeric symbol ID
// in this node's language grammar. Intended to be called once and cached.
// Returns 0 if the kind is not found.
func (n Node) SymbolForKind(kind string, named bool) uint16 {
	return n.inner.Language().IdForNodeKind(kind, named)
}

// Text returns the source text spanned by this node.
func (n Node) Text(source []byte) string {
	return n.inner.Utf8Text(source)
}

// StartByte returns the byte offset where this node starts.
func (n Node) StartByte() uint {
	return n.inner.StartByte()
}

// EndByte returns the byte offset where this node ends.
func (n Node) EndByte() uint {
	return n.inner.EndByte()
}

// StartPoint returns the (row, column) position where this node starts.
func (n Node) StartPoint() Point {
	p := n.inner.StartPosition()
	return Point{Row: p.Row, Column: p.Column}
}

// EndPoint returns the (row, column) position where this node ends.
func (n Node) EndPoint() Point {
	p := n.inner.EndPosition()
	return Point{Row: p.Row, Column: p.Column}
}

// Parent returns the node's parent, or a null node if this is the root.
func (n Node) Parent() Node {
	p := n.inner.Parent()
	return newNode(p)
}

// Child returns the i-th child (zero-based).
func (n Node) Child(i uint) Node {
	return newNode(n.inner.Child(i))
}

// ChildCount returns the total number of children (named + anonymous).
func (n Node) ChildCount() uint {
	return n.inner.ChildCount()
}

// NamedChild returns the i-th named child (zero-based).
func (n Node) NamedChild(i uint) Node {
	return newNode(n.inner.NamedChild(i))
}

// NamedChildCount returns the number of named children.
func (n Node) NamedChildCount() uint {
	return n.inner.NamedChildCount()
}

// ChildByFieldName returns the first child with the given field name.
// Note: each call allocates a C string. For hot paths, prefer
// FieldID + ChildByFieldID.
func (n Node) ChildByFieldName(name string) Node {
	return newNode(n.inner.ChildByFieldName(name))
}

// FieldID returns the numeric field ID for a field name in this node's
// language grammar. Returns 0 if the field name does not exist.
// Intended to be called once and cached.
func (n Node) FieldID(name string) uint16 {
	return n.inner.Language().FieldIdForName(name)
}

// ChildByFieldID returns the first child with the given field ID.
// This avoids the C string allocation of ChildByFieldName.
func (n Node) ChildByFieldID(id uint16) Node {
	return newNode(n.inner.ChildByFieldId(id))
}

// IsNamed returns true if this is a named node (as opposed to anonymous).
func (n Node) IsNamed() bool {
	return n.inner.IsNamed()
}

// HasError returns true if this node contains any syntax errors.
func (n Node) HasError() bool {
	return n.inner.HasError()
}

// CollectChildren returns all children (named + anonymous) as a slice.
func (n Node) CollectChildren() []Node {
	count := n.ChildCount()
	children := make([]Node, count)
	for i := range count {
		children[i] = n.Child(i)
	}
	return children
}
