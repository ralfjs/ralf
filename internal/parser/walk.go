package parser

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// WalkFunc is called for each node during tree traversal.
// Return false to skip the current node's children.
type WalkFunc func(node Node, depth int) bool

// Walk performs a depth-first traversal of the tree using a TreeCursor
// (efficient, no allocations per node). fn is called for every node,
// including anonymous ones.
func Walk(tree *Tree, fn WalkFunc) {
	cursor := tree.inner.Walk()
	defer cursor.Close()

	walk(cursor, fn, false)
}

// WalkNamed performs a depth-first traversal visiting only named nodes.
func WalkNamed(tree *Tree, fn WalkFunc) {
	cursor := tree.inner.Walk()
	defer cursor.Close()

	walk(cursor, fn, true)
}

func walk(cursor *tree_sitter.TreeCursor, fn WalkFunc, namedOnly bool) {
	visitedChildren := false

	for {
		if !visitedChildren {
			node := newNode(cursor.Node())

			skip := namedOnly && !node.IsNamed()
			if !skip {
				descend := fn(node, int(cursor.Depth()))
				if !descend {
					visitedChildren = true
					continue
				}
			}

			if cursor.GotoFirstChild() {
				continue
			}
		}

		if cursor.GotoNextSibling() {
			visitedChildren = false
			continue
		}

		if !cursor.GotoParent() {
			break
		}

		visitedChildren = true
	}
}
