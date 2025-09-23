package trie

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/relab/hotstuff"
)

func TestBasicTrieOperations(t *testing.T) {
	trie := NewMerklePatriciaTrie()
	
	// Test empty trie
	emptyRoot := trie.Root()
	if emptyRoot != (hotstuff.Hash{}) {
		t.Errorf("Empty trie should have zero hash root")
	}
	
	// Test put and get
	key := []byte("hello")
	value := []byte("world")
	
	err := trie.Put(key, value)
	if err != nil {
		t.Fatalf("Failed to put key-value: %v", err)
	}
	
	retrievedValue, found := trie.Get(key)
	if !found {
		t.Error("Key not found after put")
	}
	
	if string(retrievedValue) != string(value) {
		t.Errorf("Expected value %s, got %s", string(value), string(retrievedValue))
	}
	
	// Test non-existent key
	_, found = trie.Get([]byte("nonexistent"))
	if found {
		t.Error("Non-existent key should not be found")
	}
	
	// Test root changed
	newRoot := trie.Root()
	if newRoot == emptyRoot {
		t.Error("Root should change after inserting data")
	}
	
	t.Logf("Trie after basic operations: %s", trie.String())
}

func TestTrieMultipleKeys(t *testing.T) {
	trie := NewMerklePatriciaTrie()
	
	// Test data
	testData := map[string]string{
		"key1":    "value1",
		"key2":    "value2",
		"key123":  "value123",
		"key1234": "value1234",
		"abc":     "xyz",
		"abcd":    "wxyz",
	}
	
	// Insert all keys
	for key, value := range testData {
		err := trie.Put([]byte(key), []byte(value))
		if err != nil {
			t.Fatalf("Failed to put %s: %v", key, err)
		}
	}
	
	// Verify all keys
	for key, expectedValue := range testData {
		retrievedValue, found := trie.Get([]byte(key))
		if !found {
			t.Errorf("Key %s not found", key)
			continue
		}
		
		if string(retrievedValue) != expectedValue {
			t.Errorf("Key %s: expected %s, got %s", key, expectedValue, string(retrievedValue))
		}
	}
	
	// Test stats
	stats := trie.Stats()
	t.Logf("Trie stats: %+v", stats)
	
	if stats.LeafCount == 0 {
		t.Error("Should have leaf nodes")
	}
}

func TestTrieUpdate(t *testing.T) {
	trie := NewMerklePatriciaTrie()
	
	key := []byte("updatekey")
	value1 := []byte("value1")
	value2 := []byte("value2")
	
	// Insert initial value
	err := trie.Put(key, value1)
	if err != nil {
		t.Fatalf("Failed to put initial value: %v", err)
	}
	
	root1 := trie.Root()
	
	// Update value
	err = trie.Put(key, value2)
	if err != nil {
		t.Fatalf("Failed to update value: %v", err)
	}
	
	root2 := trie.Root()
	
	// Verify update
	retrievedValue, found := trie.Get(key)
	if !found {
		t.Error("Key not found after update")
	}
	
	if string(retrievedValue) != string(value2) {
		t.Errorf("Expected updated value %s, got %s", string(value2), string(retrievedValue))
	}
	
	// Root should change
	if root1 == root2 {
		t.Error("Root should change after update")
	}
	
	t.Logf("Root before update: %x", root1)
	t.Logf("Root after update: %x", root2)
}

func TestTrieDelete(t *testing.T) {
	trie := NewMerklePatriciaTrie()
	
	// Insert test data
	testKeys := []string{"delete1", "delete2", "delete3"}
	for _, key := range testKeys {
		err := trie.Put([]byte(key), []byte("value_"+key))
		if err != nil {
			t.Fatalf("Failed to put key %s: %v", key, err)
		}
	}
	
	// Verify all keys exist
	for _, key := range testKeys {
		_, found := trie.Get([]byte(key))
		if !found {
			t.Errorf("Key %s should exist", key)
		}
	}
	
	// Delete one key
	deleteKey := testKeys[1]
	err := trie.Delete([]byte(deleteKey))
	if err != nil {
		t.Fatalf("Failed to delete key %s: %v", deleteKey, err)
	}
	
	// Verify deleted key is gone
	_, found := trie.Get([]byte(deleteKey))
	if found {
		t.Errorf("Key %s should be deleted", deleteKey)
	}
	
	// Verify other keys still exist
	for _, key := range testKeys {
		if key == deleteKey {
			continue
		}
		_, found := trie.Get([]byte(key))
		if !found {
			t.Errorf("Key %s should still exist", key)
		}
	}
}

func TestTrieProof(t *testing.T) {
	trie := NewMerklePatriciaTrie()
	
	// Insert test data
	key := []byte("proofkey")
	value := []byte("proofvalue")
	
	err := trie.Put(key, value)
	if err != nil {
		t.Fatalf("Failed to put key: %v", err)
	}
	
	// Generate proof
	proof, err := trie.Prove(key)
	if err != nil {
		t.Fatalf("Failed to generate proof: %v", err)
	}
	
	if len(proof) == 0 {
		t.Error("Proof should not be empty")
	}
	
	// Verify proof (simplified)
	root := trie.Root()
	isValid := VerifyProof(root, key, value, proof)
	if !isValid {
		t.Error("Proof verification failed")
	}
	
	t.Logf("Generated proof with %d elements for key %s", len(proof), string(key))
}

func TestTrieCopy(t *testing.T) {
	trie := NewMerklePatriciaTrie()
	
	// Insert test data
	testData := map[string]string{
		"copy1": "value1",
		"copy2": "value2",
		"copy3": "value3",
	}
	
	for key, value := range testData {
		err := trie.Put([]byte(key), []byte(value))
		if err != nil {
			t.Fatalf("Failed to put key %s: %v", key, err)
		}
	}
	
	originalRoot := trie.Root()
	
	// Create copy
	trieCopy := trie.Copy()
	copyRoot := trieCopy.Root()
	
	// Roots should be the same
	if originalRoot != copyRoot {
		t.Error("Copy should have the same root as original")
	}
	
	// Verify copy has all data
	for key, expectedValue := range testData {
		retrievedValue, found := trieCopy.Get([]byte(key))
		if !found {
			t.Errorf("Key %s not found in copy", key)
			continue
		}
		
		if string(retrievedValue) != expectedValue {
			t.Errorf("Copy key %s: expected %s, got %s", key, expectedValue, string(retrievedValue))
		}
	}
	
	// Modify copy and verify original is unchanged
	copyKey := "copyonly"
	err := trieCopy.Put([]byte(copyKey), []byte("copyvalue"))
	if err != nil {
		t.Fatalf("Failed to put key in copy: %v", err)
	}
	
	// Copy should have new key
	_, found := trieCopy.Get([]byte(copyKey))
	if !found {
		t.Error("Copy should have new key")
	}
	
	// Original should not have new key
	_, found = trie.Get([]byte(copyKey))
	if found {
		t.Error("Original should not have new key")
	}
	
	// Roots should now be different
	newCopyRoot := trieCopy.Root()
	if originalRoot == newCopyRoot {
		t.Error("Copy root should change after modification")
	}
}

func TestTrieWithNibbleKeys(t *testing.T) {
	trie := NewMerklePatriciaTrie()
	
	// Test with keys that will create different nibble patterns
	testCases := []struct {
		key   string
		value string
	}{
		{"0", "zero"},
		{"00", "double_zero"},
		{"01", "zero_one"},
		{"10", "one_zero"},
		{"0123456789abcdef", "hex_sequence"},
		{"fedcba9876543210", "reverse_hex"},
	}
	
	// Insert all test cases
	for _, tc := range testCases {
		nibbles, err := nibblesFromHex(tc.key)
		if err != nil {
			t.Fatalf("Failed to convert hex %s to nibbles: %v", tc.key, err)
		}
		
		err = trie.Put(nibbles, []byte(tc.value))
		if err != nil {
			t.Fatalf("Failed to put hex key %s: %v", tc.key, err)
		}
	}
	
	// Verify all test cases
	for _, tc := range testCases {
		nibbles, err := nibblesFromHex(tc.key)
		if err != nil {
			t.Fatalf("Failed to convert hex %s to nibbles: %v", tc.key, err)
		}
		
		retrievedValue, found := trie.Get(nibbles)
		if !found {
			t.Errorf("Hex key %s not found", tc.key)
			continue
		}
		
		if string(retrievedValue) != tc.value {
			t.Errorf("Hex key %s: expected %s, got %s", tc.key, tc.value, string(retrievedValue))
		}
	}
	
	t.Logf("Successfully handled %d nibble-based keys", len(testCases))
}

func TestTrieStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}
	
	trie := NewMerklePatriciaTrie()
	
	// Generate random test data
	rand.Seed(time.Now().UnixNano())
	numOperations := 1000
	keys := make([][]byte, 0, numOperations)
	values := make([][]byte, 0, numOperations)
	
	// Insert phase
	t.Logf("Inserting %d random key-value pairs", numOperations)
	start := time.Now()
	
	for i := 0; i < numOperations; i++ {
		key := make([]byte, 8+rand.Intn(32))
		value := make([]byte, 16+rand.Intn(64))
		
		rand.Read(key)
		rand.Read(value)
		
		err := trie.Put(key, value)
		if err != nil {
			t.Fatalf("Failed to put key %d: %v", i, err)
		}
		
		keys = append(keys, key)
		values = append(values, value)
	}
	
	insertDuration := time.Since(start)
	t.Logf("Insert phase completed in %v", insertDuration)
	
	// Verification phase
	t.Logf("Verifying %d keys", len(keys))
	start = time.Now()
	
	for i, key := range keys {
		retrievedValue, found := trie.Get(key)
		if !found {
			t.Errorf("Key %d not found", i)
			continue
		}
		
		if string(retrievedValue) != string(values[i]) {
			t.Errorf("Key %d: value mismatch", i)
		}
	}
	
	verifyDuration := time.Since(start)
	t.Logf("Verification phase completed in %v", verifyDuration)
	
	// Stats
	stats := trie.Stats()
	t.Logf("Final trie stats: %+v", stats)
	t.Logf("Average insert time: %v", insertDuration/time.Duration(numOperations))
	t.Logf("Average lookup time: %v", verifyDuration/time.Duration(numOperations))
}

func TestNodeTypes(t *testing.T) {
	// Test individual node types
	
	// Test leaf node
	leaf := NewLeafNode([]byte{1, 2, 3}, []byte("leaf_value"))
	if leaf.Type() != LeafNode {
		t.Error("Leaf node type incorrect")
	}
	
	hash := leaf.Hash()
	if hash == (hotstuff.Hash{}) {
		t.Error("Leaf node should have non-zero hash")
	}
	
	encoded := leaf.Encode()
	if len(encoded) == 0 {
		t.Error("Leaf node encoding should not be empty")
	}
	
	t.Logf("Leaf node: %s", leaf.String())
	
	// Test branch node
	branch := NewBranchNode()
	if branch.Type() != BranchNode {
		t.Error("Branch node type incorrect")
	}
	
	branch.SetChild(5, leaf)
	child := branch.GetChild(5)
	if child != leaf {
		t.Error("Branch node child retrieval failed")
	}
	
	branch.SetValue([]byte("branch_value"))
	
	t.Logf("Branch node: %s", branch.String())
	
	// Test extension node
	ext := NewExtensionNode([]byte{4, 5, 6}, leaf)
	if ext.Type() != ExtensionNode {
		t.Error("Extension node type incorrect")
	}
	
	t.Logf("Extension node: %s", ext.String())
	
	// Test empty node
	if EmptyNodeInstance.Type() != EmptyNode {
		t.Error("Empty node type incorrect")
	}
	
	emptyHash := EmptyNodeInstance.Hash()
	if emptyHash != (hotstuff.Hash{}) {
		t.Error("Empty node should have zero hash")
	}
	
	t.Logf("Empty node: %s", EmptyNodeInstance.String())
}

func TestUtilityFunctions(t *testing.T) {
	// Test keyToNibbles
	key := []byte{0x12, 0x34, 0xAB}
	nibbles := keyToNibbles(key)
	expected := []byte{1, 2, 3, 4, 10, 11}
	
	if !nibblesEqual(nibbles, expected) {
		t.Errorf("keyToNibbles failed: expected %v, got %v", expected, nibbles)
	}
	
	// Test nibblesFromHex
	hexStr := "1234ab"
	nibbles2, err := nibblesFromHex(hexStr)
	if err != nil {
		t.Fatalf("nibblesFromHex failed: %v", err)
	}
	
	if !nibblesEqual(nibbles, nibbles2) {
		t.Errorf("nibblesFromHex mismatch: expected %v, got %v", nibbles, nibbles2)
	}
	
	// Test commonPrefix
	a := []byte{1, 2, 3, 4, 5}
	b := []byte{1, 2, 3, 7, 8}
	commonLen := commonPrefix(a, b)
	if commonLen != 3 {
		t.Errorf("commonPrefix failed: expected 3, got %d", commonLen)
	}
	
	// Test nibble validation
	if !isNibbleValid(15) {
		t.Error("15 should be a valid nibble")
	}
	
	if isNibbleValid(16) {
		t.Error("16 should not be a valid nibble")
	}
}

// Benchmark tests
func BenchmarkTriePut(b *testing.B) {
	trie := NewMerklePatriciaTrie()
	
	keys := make([][]byte, b.N)
	values := make([][]byte, b.N)
	
	for i := 0; i < b.N; i++ {
		keys[i] = []byte(fmt.Sprintf("benchmark_key_%d", i))
		values[i] = []byte(fmt.Sprintf("benchmark_value_%d", i))
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		trie.Put(keys[i], values[i])
	}
}

func BenchmarkTrieGet(b *testing.B) {
	trie := NewMerklePatriciaTrie()
	
	// Prepare data
	numKeys := 1000
	keys := make([][]byte, numKeys)
	
	for i := 0; i < numKeys; i++ {
		keys[i] = []byte(fmt.Sprintf("benchmark_key_%d", i))
		value := []byte(fmt.Sprintf("benchmark_value_%d", i))
		trie.Put(keys[i], value)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		key := keys[i%numKeys]
		trie.Get(key)
	}
}

func BenchmarkTrieRoot(b *testing.B) {
	trie := NewMerklePatriciaTrie()
	
	// Add some data
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("key_%d", i))
		value := []byte(fmt.Sprintf("value_%d", i))
		trie.Put(key, value)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		trie.Root()
	}
}
