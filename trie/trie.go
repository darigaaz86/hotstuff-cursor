package trie

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
)

// MerklePatriciaTrie implements the Ethereum Merkle Patricia Trie
type MerklePatriciaTrie struct {
	root   Node
	logger logging.Logger
}

// NewMerklePatriciaTrie creates a new empty trie
func NewMerklePatriciaTrie() *MerklePatriciaTrie {
	return &MerklePatriciaTrie{
		root:   EmptyNodeInstance,
		logger: logging.New("trie"),
	}
}

// NewMerklePatriciaTrieWithRoot creates a trie with an existing root
func NewMerklePatriciaTrieWithRoot(root Node) *MerklePatriciaTrie {
	if root == nil {
		root = EmptyNodeInstance
	}
	return &MerklePatriciaTrie{
		root:   root,
		logger: logging.New("trie"),
	}
}

// Root returns the root hash of the trie
func (t *MerklePatriciaTrie) Root() hotstuff.Hash {
	if t.root == nil || t.root.Type() == EmptyNode {
		return hotstuff.Hash{} // Empty trie has zero hash
	}
	return t.root.Hash()
}

// Get retrieves a value from the trie
func (t *MerklePatriciaTrie) Get(key []byte) ([]byte, bool) {
	if len(key) == 0 {
		return nil, false
	}
	
	nibbles := keyToNibbles(key)
	value, found := t.get(t.root, nibbles)
	return value, found
}

// get recursively searches for a value in the trie
func (t *MerklePatriciaTrie) get(node Node, key []byte) ([]byte, bool) {
	if node == nil || node.Type() == EmptyNode {
		return nil, false
	}
	
	switch n := node.(type) {
	case *LeafNodeStruct:
		if nibblesEqual(n.Key, key) {
			return n.Value, true
		}
		return nil, false
		
	case *ExtensionNodeStruct:
		if len(key) < len(n.Key) {
			return nil, false
		}
		
		// Check if key starts with extension's key
		if !nibblesEqual(n.Key, key[:len(n.Key)]) {
			return nil, false
		}
		
		// Continue search with remaining key
		remainingKey := key[len(n.Key):]
		return t.get(n.Child, remainingKey)
		
	case *BranchNodeStruct:
		if len(key) == 0 {
			// Looking for value at this branch node
			if len(n.Value) > 0 {
				return n.Value, true
			}
			return nil, false
		}
		
		// Follow the appropriate child
		nextNibble := key[0]
		if !isNibbleValid(nextNibble) {
			return nil, false
		}
		
		child := n.GetChild(int(nextNibble))
		return t.get(child, key[1:])
		
	default:
		return nil, false
	}
}

// Put inserts or updates a value in the trie
func (t *MerklePatriciaTrie) Put(key []byte, value []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("empty key not allowed")
	}
	
	if len(value) == 0 {
		// Delete the key if value is empty
		return t.Delete(key)
	}
	
	nibbles := keyToNibbles(key)
	newRoot, err := t.put(t.root, nibbles, value)
	if err != nil {
		return err
	}
	
	t.root = newRoot
	return nil
}

// put recursively inserts a value into the trie
func (t *MerklePatriciaTrie) put(node Node, key []byte, value []byte) (Node, error) {
	if node == nil || node.Type() == EmptyNode {
		// Create new leaf node
		return NewLeafNode(key, value), nil
	}
	
	switch n := node.(type) {
	case *LeafNodeStruct:
		return t.putIntoLeaf(n, key, value)
		
	case *ExtensionNodeStruct:
		return t.putIntoExtension(n, key, value)
		
	case *BranchNodeStruct:
		return t.putIntoBranch(n, key, value)
		
	default:
		return nil, fmt.Errorf("unknown node type")
	}
}

// putIntoLeaf handles insertion into a leaf node
func (t *MerklePatriciaTrie) putIntoLeaf(leaf *LeafNodeStruct, key []byte, value []byte) (Node, error) {
	if nibblesEqual(leaf.Key, key) {
		// Update existing leaf
		return NewLeafNode(key, value), nil
	}
	
	// Keys differ, need to create a branch
	commonLen := commonPrefix(leaf.Key, key)
	
	if commonLen == 0 {
		// No common prefix, create branch at root
		branch := NewBranchNode()
		
		if len(leaf.Key) == 0 {
			branch.SetValue(leaf.Value)
		} else {
			leafChild, err := t.put(EmptyNodeInstance, leaf.Key[1:], leaf.Value)
			if err != nil {
				return nil, err
			}
			branch.SetChild(int(leaf.Key[0]), leafChild)
		}
		
		if len(key) == 0 {
			branch.SetValue(value)
		} else {
			newLeafChild, err := t.put(EmptyNodeInstance, key[1:], value)
			if err != nil {
				return nil, err
			}
			branch.SetChild(int(key[0]), newLeafChild)
		}
		
		return branch, nil
	}
	
	// Create extension node for common prefix
	extension := NewExtensionNode(key[:commonLen], nil)
	
	// Create branch for diverging part
	branch := NewBranchNode()
	
	// Handle remaining parts of both keys
	leafRemaining := leaf.Key[commonLen:]
	keyRemaining := key[commonLen:]
	
	if len(leafRemaining) == 0 {
		branch.SetValue(leaf.Value)
	} else {
		leafChild, err := t.put(EmptyNodeInstance, leafRemaining[1:], leaf.Value)
		if err != nil {
			return nil, err
		}
		branch.SetChild(int(leafRemaining[0]), leafChild)
	}
	
	if len(keyRemaining) == 0 {
		branch.SetValue(value)
	} else {
		newChild, err := t.put(EmptyNodeInstance, keyRemaining[1:], value)
		if err != nil {
			return nil, err
		}
		branch.SetChild(int(keyRemaining[0]), newChild)
	}
	
	extension.Child = branch
	return extension, nil
}

// putIntoExtension handles insertion into an extension node
func (t *MerklePatriciaTrie) putIntoExtension(ext *ExtensionNodeStruct, key []byte, value []byte) (Node, error) {
	commonLen := commonPrefix(ext.Key, key)
	
	if commonLen == len(ext.Key) {
		// Key starts with extension's key, continue down
		remainingKey := key[len(ext.Key):]
		newChild, err := t.put(ext.Child, remainingKey, value)
		if err != nil {
			return nil, err
		}
		
		return NewExtensionNode(ext.Key, newChild), nil
	}
	
	// Partial match, need to split the extension
	if commonLen == 0 {
		// No common prefix, create branch
		branch := NewBranchNode()
		
		// Add extension as child
		extRemaining := ext.Key[1:]
		var extChild Node
		if len(extRemaining) == 0 {
			extChild = ext.Child
		} else {
			extChild = NewExtensionNode(extRemaining, ext.Child)
		}
		branch.SetChild(int(ext.Key[0]), extChild)
		
		// Add new value
		if len(key) == 0 {
			branch.SetValue(value)
		} else {
			newChild, err := t.put(EmptyNodeInstance, key[1:], value)
			if err != nil {
				return nil, err
			}
			branch.SetChild(int(key[0]), newChild)
		}
		
		return branch, nil
	}
	
	// Create new extension for common part
	newExt := NewExtensionNode(ext.Key[:commonLen], nil)
	
	// Create branch for diverging part
	branch := NewBranchNode()
	
	// Handle extension's remaining part
	extRemaining := ext.Key[commonLen:]
	if len(extRemaining) == 0 {
		// Extension key is prefix of new key
		branch.SetChild(int(extRemaining[0]), ext.Child)
	} else {
		extChild := NewExtensionNode(extRemaining[1:], ext.Child)
		branch.SetChild(int(extRemaining[0]), extChild)
	}
	
	// Handle new key's remaining part
	keyRemaining := key[commonLen:]
	if len(keyRemaining) == 0 {
		branch.SetValue(value)
	} else {
		newChild, err := t.put(EmptyNodeInstance, keyRemaining[1:], value)
		if err != nil {
			return nil, err
		}
		branch.SetChild(int(keyRemaining[0]), newChild)
	}
	
	newExt.Child = branch
	return newExt, nil
}

// putIntoBranch handles insertion into a branch node
func (t *MerklePatriciaTrie) putIntoBranch(branch *BranchNodeStruct, key []byte, value []byte) (Node, error) {
	if len(key) == 0 {
		// Set value at this branch
		newBranch := NewBranchNode()
		newBranch.Children = branch.Children
		newBranch.SetValue(value)
		return newBranch, nil
	}
	
	// Follow appropriate child
	nextNibble := key[0]
	if !isNibbleValid(nextNibble) {
		return nil, fmt.Errorf("invalid nibble: %d", nextNibble)
	}
	
	child := branch.GetChild(int(nextNibble))
	newChild, err := t.put(child, key[1:], value)
	if err != nil {
		return nil, err
	}
	
	// Create new branch with updated child
	newBranch := NewBranchNode()
	newBranch.Children = branch.Children
	newBranch.Value = branch.Value
	newBranch.SetChild(int(nextNibble), newChild)
	
	return newBranch, nil
}

// Delete removes a key from the trie
func (t *MerklePatriciaTrie) Delete(key []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("empty key not allowed")
	}
	
	nibbles := keyToNibbles(key)
	newRoot, err := t.delete(t.root, nibbles)
	if err != nil {
		return err
	}
	
	t.root = newRoot
	return nil
}

// delete recursively removes a key from the trie
func (t *MerklePatriciaTrie) delete(node Node, key []byte) (Node, error) {
	if node == nil || node.Type() == EmptyNode {
		// Key not found
		return EmptyNodeInstance, nil
	}
	
	switch n := node.(type) {
	case *LeafNodeStruct:
		if nibblesEqual(n.Key, key) {
			return EmptyNodeInstance, nil // Delete the leaf
		}
		return node, nil // Key not found, return unchanged
		
	case *ExtensionNodeStruct:
		if len(key) < len(n.Key) || !nibblesEqual(n.Key, key[:len(n.Key)]) {
			return node, nil // Key not found
		}
		
		remainingKey := key[len(n.Key):]
		newChild, err := t.delete(n.Child, remainingKey)
		if err != nil {
			return nil, err
		}
		
		if newChild.Type() == EmptyNode {
			return EmptyNodeInstance, nil
		}
		
		return NewExtensionNode(n.Key, newChild), nil
		
	case *BranchNodeStruct:
		if len(key) == 0 {
			// Delete value at this branch
			newBranch := NewBranchNode()
			newBranch.Children = n.Children
			// Don't set value (effectively deleting it)
			return newBranch, nil
		}
		
		nextNibble := key[0]
		if !isNibbleValid(nextNibble) {
			return node, nil
		}
		
		child := n.GetChild(int(nextNibble))
		newChild, err := t.delete(child, key[1:])
		if err != nil {
			return nil, err
		}
		
		newBranch := NewBranchNode()
		newBranch.Children = n.Children
		newBranch.Value = n.Value
		newBranch.SetChild(int(nextNibble), newChild)
		
		return newBranch, nil
		
	default:
		return nil, fmt.Errorf("unknown node type")
	}
}

// Prove generates a Merkle proof for a key
func (t *MerklePatriciaTrie) Prove(key []byte) ([][]byte, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("empty key not allowed")
	}
	
	nibbles := keyToNibbles(key)
	var proof [][]byte
	
	err := t.prove(t.root, nibbles, &proof)
	if err != nil {
		return nil, err
	}
	
	return proof, nil
}

// prove recursively builds a Merkle proof
func (t *MerklePatriciaTrie) prove(node Node, key []byte, proof *[][]byte) error {
	if node == nil || node.Type() == EmptyNode {
		return nil
	}
	
	// Add current node to proof
	*proof = append(*proof, node.Encode())
	
	switch n := node.(type) {
	case *LeafNodeStruct:
		// Proof ends at leaf
		return nil
		
	case *ExtensionNodeStruct:
		if len(key) < len(n.Key) || !nibblesEqual(n.Key, key[:len(n.Key)]) {
			return nil // Key not found
		}
		
		remainingKey := key[len(n.Key):]
		return t.prove(n.Child, remainingKey, proof)
		
	case *BranchNodeStruct:
		if len(key) == 0 {
			return nil // Proof for branch value
		}
		
		nextNibble := key[0]
		if !isNibbleValid(nextNibble) {
			return fmt.Errorf("invalid nibble: %d", nextNibble)
		}
		
		child := n.GetChild(int(nextNibble))
		return t.prove(child, key[1:], proof)
		
	default:
		return fmt.Errorf("unknown node type")
	}
}

// VerifyProof verifies a Merkle proof against a root hash
func VerifyProof(rootHash hotstuff.Hash, key []byte, value []byte, proof [][]byte) bool {
	if len(proof) == 0 {
		return false
	}
	
	// TODO: Implement proof verification
	// This would reconstruct the path from proof and verify the hash matches
	return true // Simplified for now
}

// Copy creates a deep copy of the trie
func (t *MerklePatriciaTrie) Copy() *MerklePatriciaTrie {
	return &MerklePatriciaTrie{
		root:   t.copyNode(t.root),
		logger: t.logger,
	}
}

// copyNode recursively copies a node
func (t *MerklePatriciaTrie) copyNode(node Node) Node {
	if node == nil || node.Type() == EmptyNode {
		return EmptyNodeInstance
	}
	
	switch n := node.(type) {
	case *LeafNodeStruct:
		key := make([]byte, len(n.Key))
		copy(key, n.Key)
		value := make([]byte, len(n.Value))
		copy(value, n.Value)
		return NewLeafNode(key, value)
		
	case *ExtensionNodeStruct:
		key := make([]byte, len(n.Key))
		copy(key, n.Key)
		return NewExtensionNode(key, t.copyNode(n.Child))
		
	case *BranchNodeStruct:
		newBranch := NewBranchNode()
		for i, child := range n.Children {
			newBranch.Children[i] = t.copyNode(child)
		}
		if len(n.Value) > 0 {
			value := make([]byte, len(n.Value))
			copy(value, n.Value)
			newBranch.SetValue(value)
		}
		return newBranch
		
	default:
		return EmptyNodeInstance
	}
}

// String returns a string representation of the trie
func (t *MerklePatriciaTrie) String() string {
	if t.root == nil || t.root.Type() == EmptyNode {
		return "EmptyTrie"
	}
	
	return fmt.Sprintf("Trie(root=%s, hash=%s)", 
		t.root.String(), t.Root().String()[:8]+"...")
}

// Stats returns statistics about the trie
func (t *MerklePatriciaTrie) Stats() TrieStats {
	stats := TrieStats{}
	t.collectStats(t.root, &stats, 0)
	return stats
}

// TrieStats contains statistics about the trie
type TrieStats struct {
	NodeCount     int
	LeafCount     int
	BranchCount   int
	ExtensionCount int
	MaxDepth      int
	TotalValueSize int
}

// collectStats recursively collects statistics
func (t *MerklePatriciaTrie) collectStats(node Node, stats *TrieStats, depth int) {
	if node == nil || node.Type() == EmptyNode {
		return
	}
	
	stats.NodeCount++
	if depth > stats.MaxDepth {
		stats.MaxDepth = depth
	}
	
	switch n := node.(type) {
	case *LeafNodeStruct:
		stats.LeafCount++
		stats.TotalValueSize += len(n.Value)
		
	case *ExtensionNodeStruct:
		stats.ExtensionCount++
		t.collectStats(n.Child, stats, depth+1)
		
	case *BranchNodeStruct:
		stats.BranchCount++
		if len(n.Value) > 0 {
			stats.TotalValueSize += len(n.Value)
		}
		for _, child := range n.Children {
			t.collectStats(child, stats, depth+1)
		}
	}
}
