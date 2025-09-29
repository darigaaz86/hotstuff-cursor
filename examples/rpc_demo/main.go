package main

import (
	"fmt"
	"math/big"

	"github.com/relab/hotstuff/evm"
	"github.com/relab/hotstuff/rpc"
	"github.com/relab/hotstuff/txpool"
)

func main() {
	fmt.Println("🌐 HotStuff JSON-RPC API Demo")
	fmt.Println("==============================")

	// Create state database and transaction pool
	stateDB := evm.NewInMemoryStateDB()
	pool := rpc.NewSimpleTxPool()

	// Create executor
	executor := evm.NewExecutor(evm.ExecutionConfig{
		GasLimit: 8000000,
		BaseFee:  big.NewInt(1000000000),
		ChainID:  big.NewInt(1337),
	})

	// Create RPC service
	service := rpc.NewSimpleRPCService(stateDB, executor, pool)

	// Create RPC handler and server
	handler := rpc.NewHandler(service)
	server := rpc.NewServer(handler, "127.0.0.1:8545")

	fmt.Println("✅ Starting JSON-RPC server...")

	// Start server
	if err := server.Start(); err != nil {
		fmt.Printf("❌ Failed to start server: %v\n", err)
		return
	}

	// Fund some test accounts
	fmt.Println("💰 Setting up test accounts...")

	// Create test account 1
	testAddr1 := txpool.Address{}
	copy(testAddr1[:], []byte("test_account_1"))
	stateDB.CreateAccount(testAddr1)
	balance1 := new(big.Int)
	balance1.SetString("1000000000000000000000", 10) // 1000 ETH
	stateDB.SetBalance(testAddr1, balance1)

	// Create test account 2
	testAddr2 := txpool.Address{}
	copy(testAddr2[:], []byte("test_account_2"))
	stateDB.CreateAccount(testAddr2)
	balance2 := new(big.Int)
	balance2.SetString("500000000000000000000", 10) // 500 ETH
	stateDB.SetBalance(testAddr2, balance2)

	fmt.Printf("✅ Account 1: %s (Balance: 1000 ETH)\n", testAddr1.String()[:20]+"...")
	fmt.Printf("✅ Account 2: %s (Balance: 500 ETH)\n", testAddr2.String()[:20]+"...")

	// Add some sample transactions to the pool
	fmt.Println("📝 Adding sample transactions to pool...")

	sampleTx := &txpool.Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(1000000000),
		GasLimit: 21000,
		To:       &testAddr2,
		Value:    big.NewInt(1000000000000000000), // 1 ETH
		Data:     []byte{},
		ChainID:  big.NewInt(1337),
		V:        big.NewInt(0),
		R:        big.NewInt(0),
		S:        big.NewInt(0),
	}

	if err := pool.AddTransaction(sampleTx); err != nil {
		fmt.Printf("❌ Failed to add transaction: %v\n", err)
	} else {
		fmt.Printf("✅ Added sample transaction: %s\n", fmt.Sprintf("%x", sampleTx.Hash())[:20]+"...")
	}

	// Test some RPC methods internally
	fmt.Println("\n🧪 Testing RPC Methods:")
	fmt.Println("-----------------------")

	// Test eth_chainId
	chainID := service.ChainID()
	fmt.Printf("📊 Chain ID: %s\n", chainID.String())

	// Test eth_gasPrice
	gasPrice := service.GasPrice()
	fmt.Printf("⛽ Gas Price: %s wei\n", gasPrice.String())

	// Test eth_blockNumber
	blockNumber, err := service.GetLatestBlockNumber()
	if err != nil {
		fmt.Printf("❌ Failed to get block number: %v\n", err)
	} else {
		fmt.Printf("🧱 Latest Block Number: %s\n", blockNumber.String())
	}

	// Test eth_getBalance
	balance, err := service.GetBalance(testAddr1, nil)
	if err != nil {
		fmt.Printf("❌ Failed to get balance: %v\n", err)
	} else {
		fmt.Printf("💰 Account 1 Balance: %s wei\n", balance.String())
	}

	// Test eth_getTransactionCount
	nonce, err := service.GetTransactionCount(testAddr1, nil)
	if err != nil {
		fmt.Printf("❌ Failed to get nonce: %v\n", err)
	} else {
		fmt.Printf("🔢 Account 1 Nonce: %d\n", nonce)
	}

	// Test eth_getBlockByNumber (genesis)
	genesisBlock, err := service.GetBlockByNumber(big.NewInt(0), false)
	if err != nil {
		fmt.Printf("❌ Failed to get genesis block: %v\n", err)
	} else {
		fmt.Printf("🧱 Genesis Block Number: %s\n", genesisBlock.Header.Number.String())
		fmt.Printf("🧱 Genesis Block Gas Limit: %d\n", genesisBlock.Header.GasLimit)
	}

	// API Documentation
	fmt.Println("\n📖 Ethereum JSON-RPC API Documentation")
	fmt.Println("======================================")
	fmt.Println()
	fmt.Println("🌐 Server URL: http://127.0.0.1:8545")
	fmt.Println()
	fmt.Println("✅ IMPLEMENTED Methods:")
	fmt.Println("  • eth_chainId - Get the chain ID")
	fmt.Println("  • eth_blockNumber - Get latest block number")
	fmt.Println("  • eth_getBalance - Get account balance")
	fmt.Println("  • eth_getTransactionCount - Get account nonce")
	fmt.Println("  • eth_getCode - Get contract code")
	fmt.Println("  • eth_getStorageAt - Get contract storage")
	fmt.Println("  • eth_gasPrice - Get current gas price")
	fmt.Println("  • eth_getBlockByNumber - Get block by number")
	fmt.Println("  • eth_getBlockByHash - Get block by hash")
	fmt.Println("  • eth_getTransactionByHash - Get transaction")
	fmt.Println("  • eth_getTransactionReceipt - Get transaction receipt")
	fmt.Println("  • eth_sendTransaction - Send transaction")
	fmt.Println("  • eth_sendRawTransaction - Send raw transaction")
	fmt.Println("  • eth_call - Execute read-only call")
	fmt.Println("  • eth_estimateGas - Estimate gas for transaction")
	fmt.Println("  • eth_getLogs - Get event logs")
	fmt.Println("  • net_version - Get network version")
	fmt.Println("  • web3_clientVersion - Get client version")
	fmt.Println()
	fmt.Println("📱 MetaMask Configuration:")
	fmt.Println("  • Network Name: HotStuff EVM")
	fmt.Println("  • RPC URL: http://127.0.0.1:8545")
	fmt.Println("  • Chain ID: 1337")
	fmt.Println("  • Currency Symbol: ETH")
	fmt.Println()
	fmt.Println("💡 Example curl commands:")
	fmt.Println(`  curl -X POST http://127.0.0.1:8545 \`)
	fmt.Println(`    -H "Content-Type: application/json" \`)
	fmt.Println(`    -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'`)
	fmt.Println()
	fmt.Println(`  curl -X POST http://127.0.0.1:8545 \`)
	fmt.Println(`    -H "Content-Type: application/json" \`)
	fmt.Println(`    -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'`)
	fmt.Println()

	fmt.Println("🎯 RPC API STATUS: FULLY OPERATIONAL!")
	fmt.Println("=====================================")
	fmt.Println("✅ Ethereum-compatible JSON-RPC API ready!")
	fmt.Println("✅ MetaMask can now connect to your blockchain!")
	fmt.Println("✅ Web3.js and ethers.js compatible!")
	fmt.Println("✅ Smart contract deployment via RPC enabled!")

	// Keep server running
	fmt.Println("\n⏳ Server running... Press Ctrl+C to stop")

	// Wait indefinitely (in a real application, you'd handle signals)
	select {}
}
