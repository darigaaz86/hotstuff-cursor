# 🦊 MetaMask Integration Guide

This guide shows how to connect MetaMask to your HotStuff Layer 1 blockchain.

## 🚀 Quick Setup

### Step 1: Start Your Blockchain

```bash
# Make sure your blockchain is running with RPC enabled
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 2
```

### Step 2: Add Network to MetaMask

1. **Open MetaMask** and click the network dropdown (usually shows "Ethereum Mainnet")

2. **Click "Add Network"** or "Custom RPC"

3. **Enter the following details:**

   ```
   Network Name: HotStuff EVM
   New RPC URL: http://127.0.0.1:8545
   Chain ID: 1337
   Currency Symbol: ETH
   Block Explorer URL: (leave empty for now)
   ```

4. **Click "Save"**

### Step 3: Import Test Account

Since HotStuff uses simplified address derivation for demos, you can create test accounts:

1. **Click your account icon** → "Import Account"
2. **Select "Private Key"**
3. **Use a test private key** (⚠️ **NEVER use real private keys on testnets!**)

## 🧪 Testing Contract Interactions

### Deploy Contract via MetaMask

1. **Open [Remix IDE](https://remix.ethereum.org)**
2. **Create a new file** with the HotStuffToken.sol contract
3. **Compile the contract**
4. **In Deploy tab:**
   - Environment: "Injected Web3" (MetaMask)
   - Ensure HotStuff network is selected in MetaMask
   - Click "Deploy"

### Interact with Deployed Contract

```javascript
// Example Web3.js interaction
const Web3 = require('web3');
const web3 = new Web3('http://127.0.0.1:8545');

// Contract ABI (simplified)
const tokenABI = [
  {
    "inputs": [{"name": "to", "type": "address"}, {"name": "amount", "type": "uint256"}],
    "name": "transfer",
    "outputs": [{"name": "", "type": "bool"}],
    "type": "function"
  },
  {
    "inputs": [{"name": "account", "type": "address"}],
    "name": "balanceOf", 
    "outputs": [{"name": "", "type": "uint256"}],
    "type": "function"
  }
];

// Contract instance
const contract = new web3.eth.Contract(tokenABI, 'YOUR_CONTRACT_ADDRESS');

// Check balance
async function getBalance(address) {
  const balance = await contract.methods.balanceOf(address).call();
  console.log(`Balance: ${web3.utils.fromWei(balance, 'ether')} HST`);
}

// Transfer tokens
async function transfer(to, amount) {
  const accounts = await web3.eth.getAccounts();
  const result = await contract.methods.transfer(to, web3.utils.toWei(amount, 'ether'))
    .send({ from: accounts[0] });
  console.log('Transfer successful:', result.transactionHash);
}
```

## 🔧 Troubleshooting

### Common Issues

**❌ "Network Error" when connecting**

- Ensure blockchain is running with `--rpc` flag
- Check RPC address is `127.0.0.1:8545`
- Verify firewall isn't blocking the port

**❌ "Nonce too high" error**

- Reset MetaMask account: Settings → Advanced → Reset Account

**❌ "Gas estimation failed"**

- Ensure account has sufficient ETH balance
- Try manually setting gas limit (e.g., 200000)

**❌ "Invalid chain ID"**

- Double-check Chain ID is exactly `1337`
- Restart MetaMask after adding network

### Getting Test ETH

Since this is a private blockchain, you need to fund accounts manually:

```bash
# Use the demo script to fund accounts
go run examples/layer1_demo/main.go

# Or interact directly with the state database
# (This requires modifying the demo script)
```

## 🌐 Advanced Integration

### Using with Web3 Libraries

**Ethers.js Example:**

```javascript
const { ethers } = require('ethers');

// Connect to HotStuff blockchain
const provider = new ethers.providers.JsonRpcProvider('http://127.0.0.1:8545');

// Create wallet (use test private key)
const wallet = new ethers.Wallet('YOUR_PRIVATE_KEY', provider);

// Contract interaction
const contract = new ethers.Contract(contractAddress, abi, wallet);
```

### DApp Development

Your HotStuff blockchain supports all standard Ethereum DApp patterns:

- ✅ **Contract deployment** via Remix/Hardhat/Truffle
- ✅ **Event listening** with web3.eth.subscribe
- ✅ **Transaction signing** with MetaMask
- ✅ **Contract calls** (view and state-changing)
- ✅ **Gas estimation** and price queries

## 🚀 Production Considerations

For production use:

1. **Security**: Use proper key management
2. **Persistence**: Enable `--persistent` flag for data durability  
3. **Networking**: Implement P2P layer for decentralization
4. **Monitoring**: Add block explorers and analytics
5. **Scaling**: Increase replica count for higher throughput

## 📚 Next Steps

1. **Deploy more complex contracts** (DeFi protocols, NFTs, etc.)
2. **Build a frontend DApp** using React + Web3
3. **Set up a block explorer** for better visibility
4. **Create a token faucet** for easier testing
5. **Implement multi-sig wallets** for enhanced security

---

**🎉 Congratulations!** You now have a fully functional Ethereum-compatible blockchain with MetaMask integration!

Your HotStuff blockchain provides the same development experience as Ethereum mainnet, but with:

- ⚡ **Faster finality** (HotStuff consensus)
- 🔒 **Byzantine fault tolerance**
- 🎛️ **Full control** over network parameters
- 💰 **No gas costs** (for private networks)
