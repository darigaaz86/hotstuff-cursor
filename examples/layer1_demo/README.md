# 🚀 HotStuff Layer 1 Blockchain Demo

This demo shows how to run HotStuff as a complete Layer 1 EVM blockchain and deploy/interact with smart contracts.

## 🎯 What This Demo Covers

1. **Starting the Layer 1 Blockchain** with RPC API
2. **Deploying an ERC-20 Token Contract**
3. **Transferring Tokens** via contract function calls
4. **Querying Contract State** through RPC

## 📋 Prerequisites

- Go 1.19+ installed
- HotStuff blockchain compiled (`make`)
- Basic understanding of Ethereum/smart contracts

## 🏃‍♂️ Quick Start

```bash
# 1. Clone and build (if not done already)
cd /path/to/hotstuff
make

# 2. Run the demo
go run examples/layer1_demo/main.go
```

## 🔧 Manual Step-by-Step

### Step 1: Start the Blockchain

```bash
# Start HotStuff with RPC API enabled
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 2
```

This starts:

- ✅ 4 HotStuff consensus replicas  
- ✅ 2 transaction clients
- ✅ JSON-RPC server on port 8545
- ✅ EVM execution engine
- ✅ Merkle Patricia Trie state

### Step 2: Deploy Smart Contract

Use the provided demo or any Ethereum tool:

```bash
# With curl (using our demo contract)
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_sendTransaction", 
    "params": [{
      "from": "0x742f70743166a45ad1c3b0....",
      "data": "0x608060405234801561001057600080fd5b50...",
      "gas": "0x2dc6c0"
    }],
    "id": 1
  }'
```

### Step 3: Interact with Contract

```bash
# Call contract function (transfer tokens)
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_call",
    "params": [{
      "to": "0xcontract_address",
      "data": "0xa9059cbb00000000..." 
    }, "latest"],
    "id": 1
  }'
```

## 🎮 Interactive Demo Features

The demo script provides:

- 🏗️ Automatic blockchain startup
- 💰 Pre-funded test accounts
- 📄 ERC-20 token contract deployment
- 🔄 Token transfer transactions
- 📊 Balance queries and verification
- 🌐 MetaMask connection instructions

## 🔗 Next Steps

1. **Connect MetaMask**: Use RPC URL `http://127.0.0.1:8545`, Chain ID `1337`
2. **Deploy Custom Contracts**: Use Remix IDE with the RPC endpoint
3. **Build DApps**: Connect with Web3.js or Ethers.js
4. **Scale Up**: Add more replicas for higher throughput

## 📚 Learn More

- Check `examples/layer1_demo/main.go` for full source code
- See RPC API documentation in `rpc/` directory
- Explore EVM implementation in `evm/` directory
