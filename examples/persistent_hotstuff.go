// Package main demonstrates how to use HotStuff with persistent storage
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
)

func main() {
	// Create data directory
	dataDir := "./example_data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	defer os.RemoveAll(dataDir) // Clean up for example

	fmt.Println("=== HotStuff Persistent Storage Example ===")

	// Example 1: Persistent Blockchain
	fmt.Println("1. Testing Persistent Blockchain:")
	testPersistentBlockchain(dataDir)

	// Example 2: Persistent Consensus Rules
	fmt.Println("\n2. Testing Persistent Consensus Rules:")
	testPersistentConsensus(dataDir)

	fmt.Println("\n=== Example Complete ===")
}

func testPersistentBlockchain(dataDir string) {
	// Create persistent blockchain
	config := blockchain.Config{
		StorageType: blockchain.BadgerStorage,
		DataDir:     dataDir,
		DBName:      "example_blocks.db",
	}

	bc, err := blockchain.NewBlockChain(config)
	if err != nil {
		log.Fatalf("Failed to create persistent blockchain: %v", err)
	}

	// Note: In a real application, you would ensure proper cleanup
	// The blockchain will be closed when the application exits

	fmt.Printf("✓ Created persistent blockchain in: %s\n", filepath.Join(dataDir, "example_blocks.db"))

	// Create some test blocks
	genesis := hotstuff.GetGenesis()
	fmt.Printf("✓ Genesis block: %s\n", genesis.Hash().String()[:8])

	// Create a chain of blocks
	parent := genesis
	for i := 1; i <= 3; i++ {
		block := hotstuff.NewBlock(
			parent.Hash(),
			hotstuff.NewQuorumCert(nil, parent.View(), parent.Hash()),
			hotstuff.Command(fmt.Sprintf("command_%d", i)),
			parent.View()+1,
			1,
		)

		bc.Store(block)
		fmt.Printf("✓ Stored block %d: %s\n", i, block.Hash().String()[:8])

		// Verify we can retrieve it
		if retrieved, ok := bc.LocalGet(block.Hash()); ok {
			fmt.Printf("  Retrieved command: %s\n", retrieved.Command())
		} else {
			log.Printf("  Failed to retrieve block %d\n", i)
		}

		parent = block
	}

	fmt.Printf("✓ Successfully created and verified blockchain with %d blocks\n", 4)
}

func testPersistentConsensus(dataDir string) {
	// Create persistent ChainedHotStuff
	consensusRules, err := chainedhotstuff.NewPersistent(dataDir)
	if err != nil {
		log.Fatalf("Failed to create persistent consensus: %v", err)
	}

	fmt.Printf("✓ Created persistent ChainedHotStuff in: %s\n", dataDir)

	// Clean up
	if closer, ok := consensusRules.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	// We can't fully test without a complete module setup, but we can verify creation
	fmt.Printf("✓ Consensus rules created successfully\n")
	fmt.Printf("✓ Chain length: %d\n", consensusRules.ChainLength())
}

// Demo of persistence configuration
func showPersistenceOptions() {
	fmt.Println("\n=== Persistence Configuration Options ===")

	// Memory storage (original)
	fmt.Println("Memory Storage:")
	memConfig := blockchain.Config{StorageType: blockchain.MemoryStorage}
	fmt.Printf("  Config: %+v\n", memConfig)

	// Persistent storage
	fmt.Println("Persistent Storage:")
	persistConfig := blockchain.Config{
		StorageType: blockchain.BadgerStorage,
		DataDir:     "./data",
		DBName:      "hotstuff.db",
	}
	fmt.Printf("  Config: %+v\n", persistConfig)
}
