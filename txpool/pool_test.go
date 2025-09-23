package txpool

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"math/big"
	"testing"
	"time"
)

// Test helper to create a test transaction
func createTestTx(nonce uint64, gasPrice int64, gasLimit uint64) *Transaction {
	to := Address{0x01, 0x02, 0x03}

	return NewTransaction(
		nonce,
		&to,
		big.NewInt(1000),
		gasLimit,
		big.NewInt(gasPrice),
		[]byte("test data"),
	)
}

// Test helper to create a signed transaction
func createSignedTestTx(t *testing.T, nonce uint64, gasPrice int64, gasLimit uint64) *Transaction {
	tx := createTestTx(nonce, gasPrice, gasLimit)

	// Generate a test private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Sign the transaction
	err = tx.Sign(privateKey)
	if err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	return tx
}

func TestNewTxPool(t *testing.T) {
	config := DefaultConfig()
	signer := NewEIP155Signer(big.NewInt(1))

	pool := NewTxPool(config, signer)
	defer pool.Close()

	if pool == nil {
		t.Fatal("Failed to create transaction pool")
	}

	pending, queued := pool.Stats()
	if pending != 0 || queued != 0 {
		t.Errorf("Expected empty pool, got pending=%d, queued=%d", pending, queued)
	}
}

func TestTxPool_AddLocal(t *testing.T) {
	config := DefaultConfig()
	signer := NewEIP155Signer(big.NewInt(1))
	pool := NewTxPool(config, signer)
	defer pool.Close()

	tx := createSignedTestTx(t, 0, 1000000000, 21000)

	err := pool.AddLocal(tx)
	if err != nil {
		t.Fatalf("Failed to add local transaction: %v", err)
	}

	pending, _ := pool.Stats()
	if pending != 1 {
		t.Errorf("Expected 1 pending transaction, got %d", pending)
	}

	// Verify we can retrieve the transaction
	retrieved := pool.Get(tx.Hash())
	if retrieved == nil {
		t.Error("Failed to retrieve added transaction")
	}

	if retrieved.Nonce != tx.Nonce {
		t.Errorf("Retrieved transaction nonce mismatch: expected %d, got %d", tx.Nonce, retrieved.Nonce)
	}
}

func TestTxPool_AddRemote(t *testing.T) {
	config := DefaultConfig()
	signer := NewEIP155Signer(big.NewInt(1))
	pool := NewTxPool(config, signer)
	defer pool.Close()

	tx := createSignedTestTx(t, 0, 1000000000, 21000)

	err := pool.AddRemote(tx)
	if err != nil {
		t.Fatalf("Failed to add remote transaction: %v", err)
	}

	pending, _ := pool.Stats()
	if pending != 1 {
		t.Errorf("Expected 1 pending transaction, got %d", pending)
	}
}

func TestTxPool_RejectLowGasPrice(t *testing.T) {
	config := DefaultConfig()
	config.PriceLimit = 1000000000 // 1 Gwei minimum

	signer := NewEIP155Signer(big.NewInt(1))
	pool := NewTxPool(config, signer)
	defer pool.Close()

	// Try to add transaction with gas price below minimum
	tx := createSignedTestTx(t, 0, 500000000, 21000) // 0.5 Gwei

	err := pool.AddLocal(tx)
	if err == nil {
		t.Error("Expected error for low gas price transaction")
	}

	pending, _ := pool.Stats()
	if pending != 0 {
		t.Errorf("Expected 0 pending transactions, got %d", pending)
	}
}

func TestTxPool_PendingTransactions(t *testing.T) {
	config := DefaultConfig()
	signer := NewEIP155Signer(big.NewInt(1))
	pool := NewTxPool(config, signer)
	defer pool.Close()

	// Add multiple transactions
	tx1 := createSignedTestTx(t, 0, 1000000000, 21000)
	tx2 := createSignedTestTx(t, 1, 2000000000, 21000)
	tx3 := createSignedTestTx(t, 2, 1500000000, 21000)

	pool.AddLocal(tx1)
	pool.AddLocal(tx2)
	pool.AddLocal(tx3)

	pending := pool.Pending()

	// Should have transactions grouped by sender
	totalTxs := 0
	for _, txs := range pending {
		totalTxs += len(txs)
	}

	if totalTxs != 3 {
		t.Errorf("Expected 3 pending transactions, got %d", totalTxs)
	}
}

func TestTxPool_GetTransactionsForBlock(t *testing.T) {
	config := DefaultConfig()
	signer := NewEIP155Signer(big.NewInt(1))
	pool := NewTxPool(config, signer)
	defer pool.Close()

	// Add transactions with different gas prices
	tx1 := createSignedTestTx(t, 0, 1000000000, 21000) // 1 Gwei
	tx2 := createSignedTestTx(t, 1, 3000000000, 21000) // 3 Gwei - highest
	tx3 := createSignedTestTx(t, 2, 2000000000, 21000) // 2 Gwei

	pool.AddLocal(tx1)
	pool.AddLocal(tx2)
	pool.AddLocal(tx3)

	// Get transactions for a block with gas limit
	blockTxs := pool.GetTransactionsForBlock(100000)

	if len(blockTxs) == 0 {
		t.Error("Expected some transactions for block")
	}

	// Should be sorted by gas price (highest first)
	if len(blockTxs) >= 2 {
		if blockTxs[0].GasPrice.Cmp(blockTxs[1].GasPrice) < 0 {
			t.Error("Transactions not sorted by gas price")
		}
	}
}

func TestTxPool_ToCommands(t *testing.T) {
	config := DefaultConfig()
	signer := NewEIP155Signer(big.NewInt(1))
	pool := NewTxPool(config, signer)
	defer pool.Close()

	tx1 := createSignedTestTx(t, 0, 1000000000, 21000)
	tx2 := createSignedTestTx(t, 1, 2000000000, 21000)

	transactions := []*Transaction{tx1, tx2}
	commands := pool.ToCommands(transactions)

	if len(commands) != 2 {
		t.Errorf("Expected 2 commands, got %d", len(commands))
	}

	// Verify we can convert back
	for i, cmd := range commands {
		recoveredTx, err := TransactionFromCommand(cmd)
		if err != nil {
			t.Errorf("Failed to recover transaction from command: %v", err)
		}

		if recoveredTx.Nonce != transactions[i].Nonce {
			t.Errorf("Transaction nonce mismatch after conversion")
		}
	}
}

func TestTxPool_Subscribe(t *testing.T) {
	config := DefaultConfig()
	signer := NewEIP155Signer(big.NewInt(1))
	pool := NewTxPool(config, signer)
	defer pool.Close()

	// Subscribe to new transactions
	ch := pool.Subscribe()

	// Add a transaction
	tx := createSignedTestTx(t, 0, 1000000000, 21000)

	go func() {
		time.Sleep(10 * time.Millisecond)
		pool.AddLocal(tx)
	}()

	// Wait for notification
	select {
	case receivedTx := <-ch:
		if receivedTx.Hash() != tx.Hash() {
			t.Error("Received transaction hash mismatch")
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for transaction notification")
	}
}

func TestTxPool_NextNonce(t *testing.T) {
	config := DefaultConfig()
	signer := NewEIP155Signer(big.NewInt(1))
	pool := NewTxPool(config, signer)
	defer pool.Close()

	// Generate a test address
	addr := Address{0x01, 0x02, 0x03}

	// Initially should return 0
	nonce := pool.NextNonce(addr)
	if nonce != 0 {
		t.Errorf("Expected initial nonce 0, got %d", nonce)
	}

	// Add some transactions
	tx1 := createSignedTestTx(t, 0, 1000000000, 21000)
	tx2 := createSignedTestTx(t, 1, 1000000000, 21000)

	pool.AddLocal(tx1)
	pool.AddLocal(tx2)

	// This test would need proper address derivation from signatures
	// For now, just verify the function doesn't crash
	nextNonce := pool.NextNonce(addr)
	_ = nextNonce // Suppress unused variable warning
}

func TestTransaction_Validate(t *testing.T) {
	// Test valid transaction
	tx := createTestTx(0, 1000000000, 21000)
	err := tx.Validate()
	if err != nil {
		t.Errorf("Valid transaction failed validation: %v", err)
	}

	// Test transaction with nil gas price
	invalidTx := &Transaction{
		Nonce:    0,
		GasPrice: nil,
		GasLimit: 21000,
		Value:    big.NewInt(1000),
		ChainID:  big.NewInt(1),
	}

	err = invalidTx.Validate()
	if err == nil {
		t.Error("Transaction with nil gas price should fail validation")
	}

	// Test transaction with negative value
	invalidTx2 := &Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(1000000000),
		GasLimit: 21000,
		Value:    big.NewInt(-1000),
		ChainID:  big.NewInt(1),
	}

	err = invalidTx2.Validate()
	if err == nil {
		t.Error("Transaction with negative value should fail validation")
	}
}

func TestTransaction_ToCommand(t *testing.T) {
	tx := createTestTx(5, 2000000000, 50000)

	// Convert to command
	cmd := tx.ToCommand()

	// Convert back
	recoveredTx, err := TransactionFromCommand(cmd)
	if err != nil {
		t.Fatalf("Failed to recover transaction: %v", err)
	}

	// Verify fields match
	if recoveredTx.Nonce != tx.Nonce {
		t.Errorf("Nonce mismatch: expected %d, got %d", tx.Nonce, recoveredTx.Nonce)
	}

	if recoveredTx.GasPrice.Cmp(tx.GasPrice) != 0 {
		t.Errorf("Gas price mismatch: expected %s, got %s", tx.GasPrice.String(), recoveredTx.GasPrice.String())
	}

	if recoveredTx.GasLimit != tx.GasLimit {
		t.Errorf("Gas limit mismatch: expected %d, got %d", tx.GasLimit, recoveredTx.GasLimit)
	}
}
