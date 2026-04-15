package lsp

import "sync"

// docStore tracks open text documents and their current content.
type docStore struct {
	mu   sync.Mutex
	docs map[string]*openDoc // keyed by absolute file path
}

type openDoc struct {
	content []byte
}

func newDocStore() *docStore {
	return &docStore{docs: make(map[string]*openDoc)}
}

// Open registers a newly opened document.
func (ds *docStore) Open(path string, content []byte) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.docs[path] = &openDoc{content: content}
}

// Update replaces the content of an already-open document.
func (ds *docStore) Update(path string, content []byte) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if doc, ok := ds.docs[path]; ok {
		doc.content = content
	}
}

// Close removes a document from the store.
func (ds *docStore) Close(path string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	delete(ds.docs, path)
}

// Get returns a copy of the document content. The bool is false if the
// document is not open.
func (ds *docStore) Get(path string) ([]byte, bool) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	doc, ok := ds.docs[path]
	if !ok {
		return nil, false
	}
	cp := make([]byte, len(doc.content))
	copy(cp, doc.content)
	return cp, true
}
