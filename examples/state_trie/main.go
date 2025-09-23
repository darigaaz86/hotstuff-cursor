package main

import (
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/evm"
	"github.com/relab/hotstuff/trie"
	"github.com/relab/hotstuff/txpool"
)

func main() {
	fmt.Println("🌳 HotStuff Ethereum State Trie Demo")
	fmt.Println("====================================")

	// Create temporary database directory
	dbDir := "./state_trie_demo_db"
	defer os.RemoveAll(dbDir)

	// Create BadgerDB database for trie persistence
	fmt.Println("\n📁 Creating persistent trie database...")
	db, err := trie.NewBadgerTrieDB(dbDir)
	if err != nil {
		fmt.Printf("Failed to create database: %v\n", err)
		return
	}
	defer db.Close()

	// Create trie-based state database
	fmt.Println("🌳 Creating Merkle Patricia Trie state database...")
	stateDB := evm.NewTrieStateDB(db)

	// Demo 1: Basic account operations
	fmt.Println("\n💰 Demo 1: Account Management")
	fmt.Println("-----------------------------")

	// Create test accounts
	alice := createAddress("0x1111111111111111111111111111111111111111")
	bob := createAddress("0x2222222222222222222222222222222222222222")
	charlie := createAddress("0x3333333333333333333333333333333333333333")

	// Setup initial balances
	stateDB.CreateAccount(alice)
	stateDB.SetBalance(alice, big.NewInt(5000000000000000000)) // 5 ETH

	stateDB.CreateAccount(bob)
	stateDB.SetBalance(bob, big.NewInt(3000000000000000000)) // 3 ETH

	stateDB.CreateAccount(charlie)
	stateDB.SetBalance(charlie, big.NewInt(1000000000000000000)) // 1 ETH

	fmt.Printf("  Alice:   %s ETH\n", formatETH(stateDB.GetBalance(alice)))
	fmt.Printf("  Bob:     %s ETH\n", formatETH(stateDB.GetBalance(bob)))
	fmt.Printf("  Charlie: %s ETH\n", formatETH(stateDB.GetBalance(charlie)))

	// Demo 2: Transactions with nonces
	fmt.Println("\n📝 Demo 2: Transaction Simulation")
	fmt.Println("----------------------------------")

	// Alice sends 1 ETH to Bob
	fmt.Println("  📤 Alice sends 1 ETH to Bob...")
	stateDB.SubBalance(alice, big.NewInt(1000000000000000000))
	stateDB.AddBalance(bob, big.NewInt(1000000000000000000))
	stateDB.SetNonce(alice, stateDB.GetNonce(alice)+1)

	// Bob sends 0.5 ETH to Charlie
	fmt.Println("  📤 Bob sends 0.5 ETH to Charlie...")
	stateDB.SubBalance(bob, big.NewInt(500000000000000000))
	stateDB.AddBalance(charlie, big.NewInt(500000000000000000))
	stateDB.SetNonce(bob, stateDB.GetNonce(bob)+1)

	fmt.Printf("  Alice:   %s ETH (nonce: %d)\n", formatETH(stateDB.GetBalance(alice)), stateDB.GetNonce(alice))
	fmt.Printf("  Bob:     %s ETH (nonce: %d)\n", formatETH(stateDB.GetBalance(bob)), stateDB.GetNonce(bob))
	fmt.Printf("  Charlie: %s ETH (nonce: %d)\n", formatETH(stateDB.GetBalance(charlie)), stateDB.GetNonce(charlie))

	// Demo 3: Smart contract storage
	fmt.Println("\n🤖 Demo 3: Smart Contract Storage")
	fmt.Println("----------------------------------")

	// Create a contract account
	contract := createAddress("0x4444444444444444444444444444444444444444")
	stateDB.CreateAccount(contract)

	// Set some contract code (simplified)
	contractCode := []byte("contract SimpleStorage { uint256 value; }")
	stateDB.SetCode(contract, contractCode)

	// Set storage values (simulating mapping storage)
	slot0 := createHash("0x0000000000000000000000000000000000000000000000000000000000000000") // Storage slot 0
	slot1 := createHash("0x0000000000000000000000000000000000000000000000000000000000000001") // Storage slot 1

	value42 := createHash("0x000000000000000000000000000000000000000000000000000000000000002a") // 42 in hex
	value100 := createHash("0x0000000000000000000000000000000000000000000000000000000000000064") // 100 in hex

	stateDB.SetState(contract, slot0, value42)
	stateDB.SetState(contract, slot1, value100)

	fmt.Printf("  Contract: %s\n", contract.String()[:10]+"...")
	fmt.Printf("  Code size: %d bytes\n", stateDB.GetCodeSize(contract))
	storage0 := stateDB.GetState(contract, slot0)
	storage1 := stateDB.GetState(contract, slot1)
	fmt.Printf("  Storage[0]: %d\n", bytesToInt(storage0[:]))
	fmt.Printf("  Storage[1]: %d\n", bytesToInt(storage1[:]))

	// Demo 4: State snapshots and rollback
	fmt.Println("\n🔄 Demo 4: Snapshots and Rollback")
	fmt.Println("----------------------------------")

	// Take snapshot
	snapshot := stateDB.Snapshot()
	fmt.Printf("  📸 Created snapshot %d\n", snapshot)

	aliceBalanceBefore := stateDB.GetBalance(alice)
	fmt.Printf("  Alice balance before: %s ETH\n", formatETH(aliceBalanceBefore))

	// Make some changes
	stateDB.SubBalance(alice, big.NewInt(1000000000000000000)) // Alice loses 1 ETH
	stateDB.SetNonce(alice, stateDB.GetNonce(alice)+1)

	aliceBalanceAfter := stateDB.GetBalance(alice)
	fmt.Printf("  Alice balance after changes: %s ETH\n", formatETH(aliceBalanceAfter))

	// Rollback to snapshot
	fmt.Println("  ⏪ Rolling back to snapshot...")
	stateDB.RevertToSnapshot(snapshot)

	aliceBalanceReverted := stateDB.GetBalance(alice)
	fmt.Printf("  Alice balance after rollback: %s ETH\n", formatETH(aliceBalanceReverted))

	if aliceBalanceBefore.Cmp(aliceBalanceReverted) != 0 {
		fmt.Println("  ❌ Rollback failed!")
	} else {
		fmt.Println("  ✅ Rollback successful!")
	}

	// Demo 5: State root and commit
	fmt.Println("\n🏗️  Demo 5: State Root and Persistence")
	fmt.Println("-------------------------------------")

	fmt.Printf("  Current state root: %s...\n", stateDB.GetStateRoot().String()[:16])

	// Commit the state
	fmt.Println("  💾 Committing state to trie...")
	start := time.Now()
	finalStateRoot, err := stateDB.Commit()
	if err != nil {
		fmt.Printf("  ❌ Commit failed: %v\n", err)
		return
	}
	commitDuration := time.Since(start)

	fmt.Printf("  ✅ State committed in %v\n", commitDuration)
	fmt.Printf("  📋 Final state root: %s...\n", finalStateRoot.String()[:16])

	// Demo 6: State proofs
	fmt.Println("\n🔍 Demo 6: Merkle Proofs")
	fmt.Println("-------------------------")

	// Generate account proof for Alice
	fmt.Printf("  🔍 Generating account proof for Alice...\n")
	accountProof, err := stateDB.GetAccountProof(alice)
	if err != nil {
		fmt.Printf("  ❌ Failed to generate account proof: %v\n", err)
	} else {
		fmt.Printf("  ✅ Account proof generated: %d elements\n", len(accountProof))
	}

	// Generate storage proof for contract
	fmt.Printf("  🔍 Generating storage proof for contract slot 0...\n")
	storageProof, err := stateDB.GetStorageProof(contract, slot0)
	if err != nil {
		fmt.Printf("  ❌ Failed to generate storage proof: %v\n", err)
	} else {
		fmt.Printf("  ✅ Storage proof generated: %d elements\n", len(storageProof))
	}

	// Demo 7: Performance and statistics
	fmt.Println("\n📊 Demo 7: Performance Statistics")
	fmt.Println("----------------------------------")

	stats := stateDB.Stats()
	fmt.Printf("  State trie nodes: %d\n", stats.StateTrieStats.NodeCount)
	fmt.Printf("  Storage tries: %d\n", stats.StorageTries)
	fmt.Printf("  Total storage nodes: %d\n", stats.TotalStorageNodes)
	fmt.Printf("  Max trie depth: %d\n", stats.StateTrieStats.MaxDepth)

	dbStats := db.Stats()
	fmt.Printf("  Database nodes: %d\n", dbStats.NodeCount)
	fmt.Printf("  Cache hits: %d\n", dbStats.CacheHits)
	fmt.Printf("  Cache misses: %d\n", dbStats.CacheMisses)

	if dbStats.CacheHits+dbStats.CacheMisses > 0 {
		hitRate := float64(dbStats.CacheHits) / float64(dbStats.CacheHits+dbStats.CacheMisses) * 100
		fmt.Printf("  Cache hit rate: %.1f%%\n", hitRate)
	}

	// Demo 8: State reconstruction
	fmt.Println("\n🔄 Demo 8: State Reconstruction")
	fmt.Println("-------------------------------")

	// Create new state DB with the same root
	fmt.Println("  🔄 Reconstructing state from root hash...")
	newStateDB, err := evm.NewTrieStateDBWithRoot(db, finalStateRoot)
	if err != nil {
		fmt.Printf("  ❌ Failed to reconstruct state: %v\n", err)
		return
	}

	// Verify all accounts are preserved
	aliceBalance := newStateDB.GetBalance(alice)
	bobBalance := newStateDB.GetBalance(bob)
	charlieBalance := newStateDB.GetBalance(charlie)

	fmt.Printf("  Reconstructed Alice:   %s ETH\n", formatETH(aliceBalance))
	fmt.Printf("  Reconstructed Bob:     %s ETH\n", formatETH(bobBalance))
	fmt.Printf("  Reconstructed Charlie: %s ETH\n", formatETH(charlieBalance))

	// Verify contract storage
	reconstructedValue := newStateDB.GetState(contract, slot0)
	originalValue := stateDB.GetState(contract, slot0)

	if reconstructedValue == originalValue {
		fmt.Println("  ✅ Contract storage preserved!")
	} else {
		fmt.Println("  ❌ Contract storage corrupted!")
	}

	fmt.Println("\n🎯 Demo Summary")
	fmt.Println("===============")
	fmt.Println("✅ Account management with balances and nonces")
	fmt.Println("✅ Smart contract code and storage")
	fmt.Println("✅ Atomic snapshots and rollback")
	fmt.Println("✅ Cryptographic state root computation")
	fmt.Println("✅ Merkle proof generation")
	fmt.Println("✅ Persistent storage with BadgerDB")
	fmt.Println("✅ State reconstruction from root hash")
	fmt.Println("✅ Performance optimization with caching")

	fmt.Println("\n🚀 State Trie Implementation Complete!")
	fmt.Println("This provides the foundation for:")
	fmt.Println("  • Ethereum-compatible state management")
	fmt.Println("  • Light client support with proofs")
	fmt.Println("  • Efficient storage and retrieval")
	fmt.Println("  • Cryptographic integrity guarantees")
	fmt.Println("  • Scalable smart contract storage")
}

// Helper functions

func createAddress(hexStr string) txpool.Address {
	var addr txpool.Address
	fmt.Sscanf(hexStr, "0x%40x", &addr)
	return addr
}

func createHash(hexStr string) hotstuff.Hash {
	var hash hotstuff.Hash
	fmt.Sscanf(hexStr, "0x%64x", &hash)
	return hash
}

func formatETH(wei *big.Int) string {
	if wei == nil {
		return "0.000"
	}
	eth := new(big.Float).SetInt(wei)
	eth.Quo(eth, big.NewFloat(1e18))
	return fmt.Sprintf("%.3f", eth)
}

func bytesToInt(data []byte) int {
	result := 0
	for _, b := range data {
		result = result*256 + int(b)
		if result > 1000000 { // Prevent overflow for display
			break
		}
	}
	return result
}
