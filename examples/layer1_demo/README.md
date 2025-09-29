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
# Start HotStuff with RPC API enabled (CLEAN - no automatic client logs)
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 0
```

**💡 Important**: Use `--clients 0` to avoid noisy automatic command logs like:

```
INFO cli1 client/client.go:255 25674 commands sent so far
```

This starts:

- ✅ 4 HotStuff consensus replicas  
- ✅ **No automatic clients** (clean operation)
- ✅ JSON-RPC server on port 8545
- ✅ EVM execution engine
- ✅ Merkle Patricia Trie state

**Alternative with clients** (if you need automatic transaction generation):

```bash
# With automatic clients (noisy logs)
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 2
```

### Step 2: Test RPC Connection

First, verify the RPC server is working:

```bash
# Test chain ID
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
# Expected: {"jsonrpc":"2.0","result":"0x539","id":1}

# Test block number
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":2}'
# Expected: {"jsonrpc":"2.0","result":"0x0","id":2}
```

### Step 3: Deploy Smart Contract

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

### Step 4: Interact with Contract

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

## ✅ **Clean Operation Tips**

### Recommended Commands

**For clean development (no client noise):**
```bash
# Clean consensus logs only
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 0
```

**For testing with automatic transactions:**
```bash
# With transaction generation (noisy)
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 2
```

### What You'll See

**Clean operation (`--clients 0`):**
- ✅ Consensus logs: `Creating replicas...`, `Starting replicas...`
- ✅ RPC server logs: `Starting JSON-RPC server on 127.0.0.1:8545`
- ✅ **No client spam**: No "commands sent so far" messages

**With clients (`--clients 2`):**
- ⚠️ Noisy logs: `INFO cli1 client/client.go:255 25674 commands sent so far`
- ⚠️ Continuous transaction generation

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

## 🛠️ Troubleshooting

### RPC Server Not Starting
```bash
# Check if RPC flags are properly set
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --replicas 4 --clients 0

# Verify configuration
curl -X POST http://127.0.0.1:8545 -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
```

### Too Many Logs
```bash
# Use --clients 0 to eliminate automatic transaction logs
./hotstuff run --rpc --clients 0 --replicas 4

# Or filter logs (keeping only important ones)
./hotstuff run --rpc --clients 0 --replicas 4 2>&1 | grep -E "(INFO|ERROR|Starting|RPC)"
```

### Port Already in Use
```bash
# Use a different port
./hotstuff run --rpc --rpc-addr 127.0.0.1:8546 --clients 0 --replicas 4

# Or kill existing processes
pkill hotstuff
```

## 📚 Learn More

- Check `examples/layer1_demo/main.go` for full source code
- See RPC API documentation in `rpc/` directory
- Explore EVM implementation in `evm/` directory
