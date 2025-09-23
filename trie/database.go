package trie

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger/v3"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
)

// Database interface for trie persistence
type Database interface {
	Get(hash hotstuff.Hash) (Node, error)
	Put(hash hotstuff.Hash, node Node) error
	Delete(hash hotstuff.Hash) error
	Close() error
	Stats() DatabaseStats
}

// DatabaseStats contains database statistics
type DatabaseStats struct {
	NodeCount    int64
	TotalSize    int64
	CacheHits    int64
	CacheMisses  int64
}

// BadgerTrieDB implements Database using BadgerDB
type BadgerTrieDB struct {
	db     *badger.DB
	cache  *TrieCache
	logger logging.Logger
	stats  DatabaseStats
	mu     sync.RWMutex
}

// TrieCache provides LRU caching for trie nodes
type TrieCache struct {
	cache map[hotstuff.Hash]Node
	order []hotstuff.Hash
	maxSize int
	mu     sync.RWMutex
}

// NewTrieCache creates a new trie cache
func NewTrieCache(maxSize int) *TrieCache {
	return &TrieCache{
		cache:   make(map[hotstuff.Hash]Node),
		order:   make([]hotstuff.Hash, 0, maxSize),
		maxSize: maxSize,
	}
}

// Get retrieves a node from cache
func (c *TrieCache) Get(hash hotstuff.Hash) (Node, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	node, exists := c.cache[hash]
	if exists {
		// Move to front (LRU)
		c.moveToFront(hash)
	}
	return node, exists
}

// Put stores a node in cache
func (c *TrieCache) Put(hash hotstuff.Hash, node Node) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Check if already exists
	if _, exists := c.cache[hash]; exists {
		c.moveToFront(hash)
		return
	}
	
	// Evict if necessary
	if len(c.cache) >= c.maxSize {
		c.evictLRU()
	}
	
	// Add new entry
	c.cache[hash] = node
	c.order = append([]hotstuff.Hash{hash}, c.order...)
}

// moveToFront moves an item to the front of the LRU order
func (c *TrieCache) moveToFront(hash hotstuff.Hash) {
	// Find and remove from current position
	for i, h := range c.order {
		if h == hash {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	// Add to front
	c.order = append([]hotstuff.Hash{hash}, c.order...)
}

// evictLRU removes the least recently used item
func (c *TrieCache) evictLRU() {
	if len(c.order) == 0 {
		return
	}
	
	// Remove last item (LRU)
	lastIndex := len(c.order) - 1
	lruHash := c.order[lastIndex]
	
	delete(c.cache, lruHash)
	c.order = c.order[:lastIndex]
}

// Clear empties the cache
func (c *TrieCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache = make(map[hotstuff.Hash]Node)
	c.order = c.order[:0]
}

// Size returns the current cache size
func (c *TrieCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// NewBadgerTrieDB creates a new BadgerDB-backed trie database
func NewBadgerTrieDB(dbPath string) (*BadgerTrieDB, error) {
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil // Disable badger logging
	
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger database: %w", err)
	}
	
	return &BadgerTrieDB{
		db:     db,
		cache:  NewTrieCache(10000), // Cache up to 10k nodes
		logger: logging.New("trie-db"),
	}, nil
}

// Get retrieves a node from the database
func (db *BadgerTrieDB) Get(hash hotstuff.Hash) (Node, error) {
	// Check cache first
	if node, found := db.cache.Get(hash); found {
		db.mu.Lock()
		db.stats.CacheHits++
		db.mu.Unlock()
		return node, nil
	}
	
	db.mu.Lock()
	db.stats.CacheMisses++
	db.mu.Unlock()
	
	// Read from database
	var nodeData []byte
	err := db.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(hash[:])
		if err != nil {
			return err
		}
		
		nodeData, err = item.ValueCopy(nil)
		return err
	})
	
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return EmptyNodeInstance, nil
		}
		return nil, fmt.Errorf("failed to read node: %w", err)
	}
	
	// Decode node
	node, err := db.decodeNode(nodeData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode node: %w", err)
	}
	
	// Cache the node
	db.cache.Put(hash, node)
	
	return node, nil
}

// Put stores a node in the database
func (db *BadgerTrieDB) Put(hash hotstuff.Hash, node Node) error {
	if node == nil || node.Type() == EmptyNode {
		return nil // Don't store empty nodes
	}
	
	// Encode node
	nodeData := db.encodeNode(node)
	
	// Write to database
	err := db.db.Update(func(txn *badger.Txn) error {
		return txn.Set(hash[:], nodeData)
	})
	
	if err != nil {
		return fmt.Errorf("failed to write node: %w", err)
	}
	
	// Cache the node
	db.cache.Put(hash, node)
	
	db.mu.Lock()
	db.stats.NodeCount++
	db.stats.TotalSize += int64(len(nodeData))
	db.mu.Unlock()
	
	return nil
}

// Delete removes a node from the database
func (db *BadgerTrieDB) Delete(hash hotstuff.Hash) error {
	err := db.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(hash[:])
	})
	
	if err != nil && err != badger.ErrKeyNotFound {
		return fmt.Errorf("failed to delete node: %w", err)
	}
	
	// Remove from cache
	db.cache.Put(hash, EmptyNodeInstance) // Effectively removes it
	
	db.mu.Lock()
	db.stats.NodeCount--
	db.mu.Unlock()
	
	return nil
}

// Close closes the database
func (db *BadgerTrieDB) Close() error {
	db.cache.Clear()
	return db.db.Close()
}

// Stats returns database statistics
func (db *BadgerTrieDB) Stats() DatabaseStats {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.stats
}

// encodeNode serializes a node for storage
func (db *BadgerTrieDB) encodeNode(node Node) []byte {
	// Node already has an Encode method, but we add version info
	encoded := node.Encode()
	
	// Prepend version byte
	result := make([]byte, 1+len(encoded))
	result[0] = 1 // Version 1
	copy(result[1:], encoded)
	
	return result
}

// decodeNode deserializes a node from storage
func (db *BadgerTrieDB) decodeNode(data []byte) (Node, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("invalid node data: too short")
	}
	
	version := data[0]
	if version != 1 {
		return nil, fmt.Errorf("unsupported node version: %d", version)
	}
	
	nodeData := data[1:]
	nodeType := NodeType(nodeData[0])
	
	switch nodeType {
	case EmptyNode:
		return EmptyNodeInstance, nil
		
	case LeafNode:
		return db.decodeLeafNode(nodeData[1:])
		
	case ExtensionNode:
		return db.decodeExtensionNode(nodeData[1:])
		
	case BranchNode:
		return db.decodeBranchNode(nodeData[1:])
		
	default:
		return nil, fmt.Errorf("unknown node type: %d", nodeType)
	}
}

// decodeLeafNode decodes a leaf node
func (db *BadgerTrieDB) decodeLeafNode(data []byte) (*LeafNodeStruct, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("invalid leaf node data")
	}
	
	keyLen := int(data[0])
	if len(data) < 1+keyLen {
		return nil, fmt.Errorf("invalid leaf node key length")
	}
	
	key := make([]byte, keyLen)
	copy(key, data[1:1+keyLen])
	
	offset := 1 + keyLen
	if len(data) < offset+1 {
		return nil, fmt.Errorf("invalid leaf node value length")
	}
	
	valueLen := int(data[offset])
	if valueLen == 255 {
		// Large value, next 2 bytes contain length
		if len(data) < offset+3 {
			return nil, fmt.Errorf("invalid leaf node large value length")
		}
		valueLen = int(binary.BigEndian.Uint16(data[offset+1:offset+3]))
		offset += 3
	} else {
		offset++
	}
	
	if len(data) < offset+valueLen {
		return nil, fmt.Errorf("invalid leaf node value")
	}
	
	value := make([]byte, valueLen)
	copy(value, data[offset:offset+valueLen])
	
	return NewLeafNode(key, value), nil
}

// decodeExtensionNode decodes an extension node
func (db *BadgerTrieDB) decodeExtensionNode(data []byte) (*ExtensionNodeStruct, error) {
	if len(data) < 33 { // 1 byte key length + 32 bytes child hash minimum
		return nil, fmt.Errorf("invalid extension node data")
	}
	
	keyLen := int(data[0])
	if len(data) < 1+keyLen+32 {
		return nil, fmt.Errorf("invalid extension node length")
	}
	
	key := make([]byte, keyLen)
	copy(key, data[1:1+keyLen])
	
	// Child hash
	childHashBytes := data[1+keyLen:1+keyLen+32]
	var childHash hotstuff.Hash
	copy(childHash[:], childHashBytes)
	
	// For now, we'll need to resolve the child when accessed
	// This is a simplified implementation
	child := &HashNode{hash: childHash, db: db}
	
	return NewExtensionNode(key, child), nil
}

// decodeBranchNode decodes a branch node
func (db *BadgerTrieDB) decodeBranchNode(data []byte) (*BranchNodeStruct, error) {
	if len(data) < 16*32+1 { // 16 child hashes + value length
		return nil, fmt.Errorf("invalid branch node data")
	}
	
	branch := NewBranchNode()
	
	// Decode children hashes
	for i := 0; i < 16; i++ {
		childHashBytes := data[i*32:(i+1)*32]
		
		// Check if it's an empty hash
		isEmpty := true
		for _, b := range childHashBytes {
			if b != 0 {
				isEmpty = false
				break
			}
		}
		
		if !isEmpty {
			var childHash hotstuff.Hash
			copy(childHash[:], childHashBytes)
			child := &HashNode{hash: childHash, db: db}
			branch.SetChild(i, child)
		}
	}
	
	// Decode value
	valueLen := int(data[16*32])
	if valueLen > 0 {
		if len(data) < 16*32+1+valueLen {
			return nil, fmt.Errorf("invalid branch node value")
		}
		
		value := make([]byte, valueLen)
		copy(value, data[16*32+1:16*32+1+valueLen])
		branch.SetValue(value)
	}
	
	return branch, nil
}

// HashNode represents a node that hasn't been loaded yet (lazy loading)
type HashNode struct {
	hash   hotstuff.Hash
	db     *BadgerTrieDB
	cached Node
}

func (n *HashNode) Type() NodeType {
	if n.cached == nil {
		n.resolve()
	}
		if n.cached == nil {
			return EmptyNode
		}
	return n.cached.Type()
}

func (n *HashNode) Hash() hotstuff.Hash {
	return n.hash
}

func (n *HashNode) Encode() []byte {
	if n.cached == nil {
		n.resolve()
	}
	if n.cached == nil {
		return EmptyNodeInstance.Encode()
	}
	return n.cached.Encode()
}

func (n *HashNode) String() string {
	return fmt.Sprintf("HashNode(%s)", n.hash.String()[:8]+"...")
}

func (n *HashNode) resolve() {
	if n.db != nil {
		node, err := n.db.Get(n.hash)
		if err == nil {
			n.cached = node
		}
	}
}
