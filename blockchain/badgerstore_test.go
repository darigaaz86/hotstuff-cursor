package blockchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/relab/hotstuff"
)

func TestBadgerBlockChain_Basic(t *testing.T) {
	// Create temporary directory for test database
	tmpDir, err := os.MkdirTemp("", "hotstuff_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create BadgerDB blockchain
	bc, err := NewBadgerBlockChain(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create BadgerDB blockchain: %v", err)
	}
	defer func() {
		if badgerBC, ok := bc.(*badgerBlockChain); ok {
			badgerBC.Close()
		}
	}()

	// Manually ensure genesis is stored since we don't call InitModule
	badgerBC := bc.(*badgerBlockChain)
	if err := badgerBC.ensureGenesis(); err != nil {
		t.Fatalf("Failed to ensure genesis: %v", err)
	}

	// Test storing and retrieving genesis block
	genesis := hotstuff.GetGenesis()

	// Genesis should now be stored
	retrievedBlock, ok := bc.LocalGet(genesis.Hash())
	if !ok {
		t.Fatal("Genesis block should be stored")
	}

	if retrievedBlock.Hash() != genesis.Hash() {
		t.Errorf("Retrieved genesis hash %s != expected %s", retrievedBlock.Hash(), genesis.Hash())
	}

	// Create a new block
	newBlock := hotstuff.NewBlock(
		genesis.Hash(),
		hotstuff.NewQuorumCert(nil, genesis.View(), genesis.Hash()),
		"test command",
		genesis.View()+1,
		1,
	)

	// Store the new block
	bc.Store(newBlock)

	// Retrieve and verify the new block
	retrievedBlock, ok = bc.LocalGet(newBlock.Hash())
	if !ok {
		t.Fatal("Failed to retrieve stored block")
	}

	if retrievedBlock.Hash() != newBlock.Hash() {
		t.Errorf("Retrieved block hash %s != expected %s", retrievedBlock.Hash(), newBlock.Hash())
	}

	if retrievedBlock.View() != newBlock.View() {
		t.Errorf("Retrieved block view %d != expected %d", retrievedBlock.View(), newBlock.View())
	}

	if string(retrievedBlock.Command()) != string(newBlock.Command()) {
		t.Errorf("Retrieved block command %s != expected %s", retrievedBlock.Command(), newBlock.Command())
	}
}

func TestBadgerBlockChain_Persistence(t *testing.T) {
	// Create temporary directory for test database
	tmpDir, err := os.MkdirTemp("", "hotstuff_persist_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "persist.db")

	// Create and store a block
	var testBlockHash hotstuff.Hash
	{
		bc, err := NewBadgerBlockChain(dbPath)
		if err != nil {
			t.Fatalf("Failed to create BadgerDB blockchain: %v", err)
		}

		// Manually ensure genesis is stored
		badgerBC := bc.(*badgerBlockChain)
		if err := badgerBC.ensureGenesis(); err != nil {
			t.Fatalf("Failed to ensure genesis: %v", err)
		}

		genesis := hotstuff.GetGenesis()
		testBlock := hotstuff.NewBlock(
			genesis.Hash(),
			hotstuff.NewQuorumCert(nil, genesis.View(), genesis.Hash()),
			"persistence test",
			genesis.View()+1,
			1,
		)
		testBlockHash = testBlock.Hash()

		bc.Store(testBlock)

		// Close the database
		badgerBC.Close()
	}

	// Reopen the database and verify the block persisted
	{
		bc, err := NewBadgerBlockChain(dbPath)
		if err != nil {
			t.Fatalf("Failed to reopen BadgerDB blockchain: %v", err)
		}
		defer func() {
			if badgerBC, ok := bc.(*badgerBlockChain); ok {
				badgerBC.Close()
			}
		}()

		retrievedBlock, ok := bc.LocalGet(testBlockHash)
		if !ok {
			t.Fatal("Block should persist across database restarts")
		}

		if string(retrievedBlock.Command()) != "persistence test" {
			t.Errorf("Retrieved block command %s != expected 'persistence test'", retrievedBlock.Command())
		}
	}
}

func TestStateStore_Basic(t *testing.T) {
	// Create temporary directory for test database
	tmpDir, err := os.MkdirTemp("", "hotstuff_state_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create state store
	store, err := NewStateStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}
	defer store.Close()

	// Test current view
	view, err := store.GetCurrentView()
	if err != nil {
		t.Fatalf("Failed to get current view: %v", err)
	}
	if view != 1 {
		t.Errorf("Default current view should be 1, got %d", view)
	}

	// Update current view
	newView := hotstuff.View(5)
	if err := store.SetCurrentView(newView); err != nil {
		t.Fatalf("Failed to set current view: %v", err)
	}

	// Verify update
	view, err = store.GetCurrentView()
	if err != nil {
		t.Fatalf("Failed to get updated current view: %v", err)
	}
	if view != newView {
		t.Errorf("Updated current view should be %d, got %d", newView, view)
	}

	// Test last vote
	lastVote, err := store.GetLastVote()
	if err != nil {
		t.Fatalf("Failed to get last vote: %v", err)
	}
	if lastVote != 0 {
		t.Errorf("Default last vote should be 0, got %d", lastVote)
	}

	// Update last vote
	newLastVote := hotstuff.View(3)
	if err := store.SetLastVote(newLastVote); err != nil {
		t.Fatalf("Failed to set last vote: %v", err)
	}

	// Verify update
	lastVote, err = store.GetLastVote()
	if err != nil {
		t.Fatalf("Failed to get updated last vote: %v", err)
	}
	if lastVote != newLastVote {
		t.Errorf("Updated last vote should be %d, got %d", newLastVote, lastVote)
	}
}

func TestConfig_NewBlockChain(t *testing.T) {
	// Test memory storage
	memConfig := Config{StorageType: MemoryStorage}
	memBC, err := NewBlockChain(memConfig)
	if err != nil {
		t.Fatalf("Failed to create memory blockchain: %v", err)
	}
	if _, ok := memBC.(*blockChain); !ok {
		t.Error("Memory config should return in-memory blockchain")
	}

	// Test BadgerDB storage
	tmpDir, err := os.MkdirTemp("", "hotstuff_config_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	badgerConfig := Config{
		StorageType: BadgerStorage,
		DataDir:     tmpDir,
		DBName:      "config_test.db",
	}
	badgerBC, err := NewBlockChain(badgerConfig)
	if err != nil {
		t.Fatalf("Failed to create BadgerDB blockchain: %v", err)
	}
	defer func() {
		if bc, ok := badgerBC.(*badgerBlockChain); ok {
			bc.Close()
		}
	}()

	if _, ok := badgerBC.(*badgerBlockChain); !ok {
		t.Error("BadgerDB config should return BadgerDB blockchain")
	}

	// Test invalid storage type
	invalidConfig := Config{StorageType: "invalid"}
	_, err = NewBlockChain(invalidConfig)
	if err == nil {
		t.Error("Invalid storage type should return error")
	}
}
