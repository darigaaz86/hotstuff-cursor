// Package chainedhotstuff implements the pipelined three-chain version of the HotStuff protocol with persistence.
package chainedhotstuff

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

// PersistentChainedHotStuff implements the pipelined three-phase HotStuff protocol with persistent state.
type PersistentChainedHotStuff struct {
	blockChain modules.BlockChain
	logger     logging.Logger

	// Persistent state store
	stateStore *blockchain.StateStore

	// protocol variables
	bLock *hotstuff.Block // the currently locked block
}

// NewPersistent returns a new persistent chainedhotstuff instance.
func NewPersistent(dataDir string) (consensus.Rules, error) {
	stateStore, err := blockchain.NewStateStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create state store: %w", err)
	}

	return &PersistentChainedHotStuff{
		stateStore: stateStore,
		bLock:      hotstuff.GetGenesis(), // will be loaded from state store in InitModule
	}, nil
}

// InitModule initializes the module
func (hs *PersistentChainedHotStuff) InitModule(mods *modules.Core) {
	mods.Get(&hs.blockChain, &hs.logger)

	// Load persistent state
	if err := hs.loadState(); err != nil {
		hs.logger.Errorf("Failed to load persistent ChainedHotStuff state: %v", err)
		// Continue with genesis if loading fails
		hs.bLock = hotstuff.GetGenesis()
		hs.persistLock()
	}
}

// loadState loads the locked block from persistent storage
func (hs *PersistentChainedHotStuff) loadState() error {
	lockedHash, err := hs.stateStore.GetLockedBlockHash()
	if err != nil {
		return fmt.Errorf("failed to load locked block hash: %w", err)
	}

	// Get the locked block from blockchain
	if lockedBlock, ok := hs.blockChain.LocalGet(lockedHash); ok {
		hs.bLock = lockedBlock
		hs.logger.Infof("Loaded locked block: %s at view %d", lockedHash.String(), lockedBlock.View())
	} else {
		hs.logger.Warnf("Locked block %s not found in blockchain, using genesis", lockedHash.String())
		hs.bLock = hotstuff.GetGenesis()
	}

	return nil
}

// persistLock saves the locked block hash to persistent storage
func (hs *PersistentChainedHotStuff) persistLock() {
	if err := hs.stateStore.SetLockedBlockHash(hs.bLock.Hash()); err != nil {
		hs.logger.Errorf("Failed to persist locked block: %v", err)
	}
}

func (hs *PersistentChainedHotStuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return hs.blockChain.Get(qc.BlockHash())
}

// CommitRule decides whether an ancestor of the block should be committed.
func (hs *PersistentChainedHotStuff) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	block1, ok := hs.qcRef(block.QuorumCert())
	if !ok {
		return nil
	}

	// Note that we do not call UpdateHighQC here.
	// This is done through AdvanceView, which the Consensus implementation will call.
	hs.logger.Debug("PRE_COMMIT: ", block1)

	block2, ok := hs.qcRef(block1.QuorumCert())
	if !ok {
		return nil
	}

	lockUpdated := false
	if block2.View() > hs.bLock.View() {
		hs.logger.Debug("COMMIT: ", block2)
		hs.bLock = block2
		lockUpdated = true
	}

	// Persist lock update
	if lockUpdated {
		hs.persistLock()
	}

	block3, ok := hs.qcRef(block2.QuorumCert())
	if !ok {
		return nil
	}

	if block1.Parent() == block2.Hash() && block2.Parent() == block3.Hash() {
		hs.logger.Debug("DECIDE: ", block3)
		return block3
	}

	return nil
}

// VoteRule decides whether to vote for the proposal or not.
func (hs *PersistentChainedHotStuff) VoteRule(proposal hotstuff.ProposeMsg) bool {
	block := proposal.Block

	qcBlock, ok := hs.qcRef(block.QuorumCert())
	if !ok {
		hs.logger.Info("VoteRule: Could not find block referenced by QC.")
		return false
	}

	safe := qcBlock.View() > hs.bLock.View()
	if !safe {
		hs.logger.Info("VoteRule: liveness condition failed")
		return false
	}

	hs.logger.Debug("VoteRule: SUCCESS")
	return true
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (hs *PersistentChainedHotStuff) ChainLength() int {
	return 3
}

// Close cleanly shuts down the persistent ChainedHotStuff
func (hs *PersistentChainedHotStuff) Close() error {
	if hs.stateStore != nil {
		return hs.stateStore.Close()
	}
	return nil
}

var _ consensus.Rules = (*PersistentChainedHotStuff)(nil)
