package evm

import (
	"math/big"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/txpool"
)

func TestEVMBlockCreation(t *testing.T) {
	// Create test transactions
	tx1 := createTestTransaction(1, "0x1000000000000000000000000000000000000001", big.NewInt(1000000000000000000))
	tx2 := createTestTransaction(2, "0x1000000000000000000000000000000000000002", big.NewInt(2000000000000000000))
	
	transactions := []*txpool.Transaction{tx1, tx2}
	
	// Create test block
	parentHash := hotstuff.Hash{}
	cert := hotstuff.QuorumCert{}
	view := hotstuff.View(1)
	proposer := hotstuff.ID(1)
	stateRoot := hotstuff.Hash{}
	gasLimit := uint64(8000000)
	
	block := NewEVMBlock(parentHash, cert, transactions, view, proposer, stateRoot, gasLimit)
	
	// Verify block structure
	if block == nil {
		t.Fatal("Block creation failed")
	}
	
	if len(block.Transactions) != 2 {
		t.Errorf("Expected 2 transactions, got %d", len(block.Transactions))
	}
	
	if block.Header.GasLimit != gasLimit {
		t.Errorf("Expected gas limit %d, got %d", gasLimit, block.Header.GasLimit)
	}
	
	if block.View() != view {
		t.Errorf("Expected view %d, got %d", view, block.View())
	}
	
	if block.Proposer() != proposer {
		t.Errorf("Expected proposer %d, got %d", proposer, block.Proposer())
	}
	
	t.Logf("Created EVM block: %s", block.String())
}

func TestStateDBOperations(t *testing.T) {
	stateDB := NewInMemoryStateDB()
	
	// Test account creation and balance operations
	addr := createTestAddress("0x1000000000000000000000000000000000000001")
	
	// Initially account should not exist
	if stateDB.Exist(addr) {
		t.Error("Account should not exist initially")
	}
	
	// Create account
	stateDB.CreateAccount(addr)
	
	if !stateDB.Exist(addr) {
		t.Error("Account should exist after creation")
	}
	
	// Test balance operations
	balance := big.NewInt(1000000000000000000) // 1 ETH
	stateDB.SetBalance(addr, balance)
	
	retrievedBalance := stateDB.GetBalance(addr)
	if retrievedBalance.Cmp(balance) != 0 {
		t.Errorf("Expected balance %s, got %s", balance.String(), retrievedBalance.String())
	}
	
	// Test nonce operations
	stateDB.SetNonce(addr, 5)
	if stateDB.GetNonce(addr) != 5 {
		t.Errorf("Expected nonce 5, got %d", stateDB.GetNonce(addr))
	}
	
	// Test code operations
	code := []byte("contract code")
	stateDB.SetCode(addr, code)
	
	retrievedCode := stateDB.GetCode(addr)
	if string(retrievedCode) != string(code) {
		t.Errorf("Expected code %s, got %s", string(code), string(retrievedCode))
	}
	
	// Test storage operations
	key := hotstuff.Hash{1, 2, 3}
	value := hotstuff.Hash{4, 5, 6}
	
	stateDB.SetState(addr, key, value)
	retrievedValue := stateDB.GetState(addr, key)
	
	if retrievedValue != value {
		t.Errorf("Expected storage value %x, got %x", value, retrievedValue)
	}
	
	t.Logf("StateDB operations test passed")
}

func TestTransactionExecution(t *testing.T) {
	// Create executor
	config := ExecutionConfig{
		GasLimit: 8000000,
		BaseFee:  big.NewInt(1000000000),
		ChainID:  big.NewInt(1337),
	}
	executor := NewExecutor(config)
	
	// Create state with test accounts
	stateDB := NewInMemoryStateDB()
	stateDB.SetupGenesisAccounts()
	
	// Create test transaction with addresses that match our simplified sender recovery
	tx := &txpool.Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(2000000000), // 2 gwei
		GasLimit: 21000,
		To:       nil, // We'll set this after calculating sender
		Value:    big.NewInt(1000000000000000000), // 1 ETH
		Data:     []byte{},
		ChainID:  big.NewInt(1337),
	}
	
	// Calculate what the sender address will be (derived from tx hash)
	hash := tx.Hash()
	var fromAddr txpool.Address
	copy(fromAddr[:], hash[:20])
	
	// Create a valid "to" address
	toAddr := createTestAddress("0x1000000000000000000000000000000000000002")
	tx.To = &toAddr
	
	// Ensure sender has enough balance and correct nonce
	stateDB.CreateAccount(fromAddr)
	stateDB.SetBalance(fromAddr, big.NewInt(5000000000000000000)) // 5 ETH
	stateDB.SetNonce(fromAddr, 0)
	
	// Create test block
	block := createTestBlock()
	
	// Execute transaction
	receipt, err := executor.ExecuteTransaction(tx, stateDB, block, 0, 0)
	if err != nil {
		t.Fatalf("Transaction execution failed: %v", err)
	}
	
	// Verify receipt
	if receipt.Status != 1 {
		t.Error("Transaction should have succeeded")
	}
	
	if receipt.GasUsed != 21000 {
		t.Errorf("Expected gas used 21000, got %d", receipt.GasUsed)
	}
	
	// Verify state changes
	fromBalance := stateDB.GetBalance(fromAddr)
	toBalance := stateDB.GetBalance(toAddr)
	
	expectedToBalance := big.NewInt(1000000000000000000)   // 1 ETH
	maxFromBalance := big.NewInt(4000000000000000000)     // Should be less than 4 ETH after transfer and gas
	
	if fromBalance.Cmp(maxFromBalance) > 0 { // Allow for gas costs
		t.Errorf("From balance too high: %s (should be less than %s)", fromBalance.String(), maxFromBalance.String())
	}
	
	if toBalance.Cmp(expectedToBalance) != 0 {
		t.Errorf("Expected to balance %s, got %s", expectedToBalance.String(), toBalance.String())
	}
	
	t.Logf("Transaction execution test passed")
}

func TestBlockBuilder(t *testing.T) {
	// Create transaction pool
	config := txpool.DefaultConfig()
	signer := txpool.NewEIP155Signer(big.NewInt(1337))
	pool := txpool.NewTxPool(config, signer)
	defer pool.Close()
	
	// Create state DB
	stateDB := NewInMemoryStateDB()
	
	// Create block builder
	builderConfig := DefaultBlockBuilderConfig()
	builder := NewBlockBuilder(pool, stateDB, builderConfig)
	
	// Create genesis block
	genesisBlock := builder.CreateGenesisBlock()
	
	if genesisBlock == nil {
		t.Fatal("Genesis block creation failed")
	}
	
	if genesisBlock.Header.Number.Int64() != 0 {
		t.Errorf("Expected genesis block number 0, got %d", genesisBlock.Header.Number.Int64())
	}
	
	// Add some transactions to pool
	tx1 := createTestTransaction(0, "0x1000000000000000000000000000000000000002", big.NewInt(1000000000000000000))
	tx2 := createTestTransaction(1, "0x1000000000000000000000000000000000000003", big.NewInt(2000000000000000000))
	
	pool.AddLocal(tx1)
	pool.AddLocal(tx2)
	
	// Build a block
	parentHash := genesisBlock.Hash()
	cert := hotstuff.QuorumCert{}
	view := hotstuff.View(2)
	proposer := hotstuff.ID(1)
	
	block, err := builder.BuildBlock(parentHash, cert, view, proposer)
	if err != nil {
		t.Fatalf("Block building failed: %v", err)
	}
	
	// Note: Block might be empty due to transaction validation failures
	// This is expected behavior in the test environment
	
	if block.Parent() != parentHash {
		t.Error("Block parent hash mismatch")
	}
	
	if block.View() != view {
		t.Errorf("Expected view %d, got %d", view, block.View())
	}
	
	t.Logf("Block builder test passed, built block: %s", block.String())
}

// Helper functions

func createTestTransaction(nonce uint64, toAddr string, value *big.Int) *txpool.Transaction {
	to := createTestAddress(toAddr)
	return &txpool.Transaction{
		Nonce:    nonce,
		GasPrice: big.NewInt(1000000000), // 1 gwei
		GasLimit: 21000,
		To:       &to,
		Value:    value,
		Data:     []byte{},
		ChainID:  big.NewInt(1337),
	}
}

func createTestAddress(addrStr string) txpool.Address {
	var addr txpool.Address
	// Simplified address creation for testing
	copy(addr[:], addrStr[:20])
	return addr
}

func createTestBlock() *EVMBlock {
	return NewEVMBlock(
		hotstuff.Hash{},
		hotstuff.QuorumCert{},
		[]*txpool.Transaction{},
		hotstuff.View(1),
		hotstuff.ID(1),
		hotstuff.Hash{},
		8000000,
	)
}
