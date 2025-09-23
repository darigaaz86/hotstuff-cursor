package consensus

import (
	"fmt"
	"sync"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// persistentConsensusBase extends consensusBase with persistent state management
type persistentConsensusBase struct {
	impl Rules

	acceptor       modules.Acceptor
	blockChain     modules.BlockChain
	commandQueue   modules.CommandQueue
	configuration  modules.Configuration
	crypto         modules.Crypto
	eventLoop      *eventloop.EventLoop
	executor       modules.ExecutorExt
	forkHandler    modules.ForkHandlerExt
	leaderRotation modules.LeaderRotation
	logger         logging.Logger
	opts           *modules.Options
	synchronizer   modules.Synchronizer

	kauri modules.Kauri

	// Persistent state store
	stateStore *blockchain.StateStore

	// In-memory state (loaded from persistent store)
	mut      sync.Mutex
	lastVote hotstuff.View
	bExec    *hotstuff.Block
}

// NewPersistent returns a new persistent Consensus instance
func NewPersistent(impl Rules, dataDir string) (modules.Consensus, error) {
	stateStore, err := blockchain.NewStateStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create state store: %w", err)
	}

	return NewPersistentWithStateStore(impl, stateStore)
}

// NewPersistentWithStateStore creates a persistent consensus with an existing state store
func NewPersistentWithStateStore(impl Rules, stateStore *blockchain.StateStore) (modules.Consensus, error) {
	return &persistentConsensusBase{
		impl:       impl,
		stateStore: stateStore,
		bExec:      hotstuff.GetGenesis(), // will be loaded from state store in InitModule
	}, nil
}

// InitModule initializes the persistent consensus module
func (cs *persistentConsensusBase) InitModule(mods *modules.Core) {
	mods.Get(
		&cs.acceptor,
		&cs.blockChain,
		&cs.commandQueue,
		&cs.configuration,
		&cs.crypto,
		&cs.eventLoop,
		&cs.executor,
		&cs.forkHandler,
		&cs.leaderRotation,
		&cs.logger,
		&cs.opts,
		&cs.synchronizer,
	)

	mods.TryGet(&cs.kauri)

	if mod, ok := cs.impl.(modules.Module); ok {
		mod.InitModule(mods)
	}

	// Load persistent state
	if err := cs.loadState(); err != nil {
		cs.logger.Errorf("Failed to load persistent state: %v", err)
		// Continue with defaults if loading fails
	}

	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		cs.OnPropose(event.(hotstuff.ProposeMsg))
	})
}

// loadState loads the consensus state from persistent storage
func (cs *persistentConsensusBase) loadState() error {
	cs.mut.Lock()
	defer cs.mut.Unlock()

	// Load last vote
	lastVote, err := cs.stateStore.GetLastVote()
	if err != nil {
		return fmt.Errorf("failed to load last vote: %w", err)
	}
	cs.lastVote = lastVote

	// Load committed block
	committedHash, err := cs.stateStore.GetCommittedBlockHash()
	if err != nil {
		return fmt.Errorf("failed to load committed block hash: %w", err)
	}

	// Get the committed block from blockchain
	if committedBlock, ok := cs.blockChain.LocalGet(committedHash); ok {
		cs.bExec = committedBlock
	} else {
		cs.logger.Warnf("Committed block %s not found in blockchain, using genesis", committedHash.String())
		cs.bExec = hotstuff.GetGenesis()
	}

	cs.logger.Infof("Loaded persistent state: lastVote=%d, committedBlock=%s",
		cs.lastVote, cs.bExec.Hash().String())

	return nil
}

// saveState saves the consensus state to persistent storage
func (cs *persistentConsensusBase) saveState() error {
	cs.mut.Lock()
	defer cs.mut.Unlock()

	updates := map[string]interface{}{
		"last_vote":      cs.lastVote,
		"committed_hash": cs.bExec.Hash(),
	}

	return cs.stateStore.UpdateConsensusState(updates)
}

// CommittedBlock returns the committed block
func (cs *persistentConsensusBase) CommittedBlock() *hotstuff.Block {
	cs.mut.Lock()
	defer cs.mut.Unlock()
	return cs.bExec
}

// StopVoting ensures that no voting happens in a view earlier than `view`
func (cs *persistentConsensusBase) StopVoting(view hotstuff.View) {
	cs.mut.Lock()
	updated := false
	if cs.lastVote < view {
		cs.lastVote = view
		updated = true
	}
	cs.mut.Unlock()

	// Persist the change if updated
	if updated {
		if err := cs.stateStore.SetLastVote(view); err != nil {
			cs.logger.Errorf("Failed to persist last vote: %v", err)
		}
	}
}

// Propose creates a new proposal (same as base implementation)
func (cs *persistentConsensusBase) Propose(cert hotstuff.SyncInfo) {
	cs.logger.Debug("Propose")

	qc, ok := cert.QC()
	if ok {
		// tell the acceptor that the previous proposal succeeded.
		if qcBlock, ok := cs.blockChain.Get(qc.BlockHash()); ok {
			cs.acceptor.Proposed(qcBlock.Command())
		} else {
			cs.logger.Errorf("Could not find block for QC: %s", qc)
		}
	}

	ctx, cancel := synchronizer.TimeoutContext(cs.eventLoop.Context(), cs.eventLoop)
	defer cancel()

	cmd, ok := cs.commandQueue.Get(ctx)
	if !ok {
		cs.logger.Debug("Propose: No command")
		return
	}

	var proposal hotstuff.ProposeMsg
	if proposer, ok := cs.impl.(ProposeRuler); ok {
		proposal, ok = proposer.ProposeRule(cert, cmd)
		if !ok {
			cs.logger.Debug("Propose: No block")
			return
		}
	} else {
		proposal = hotstuff.ProposeMsg{
			ID: cs.opts.ID(),
			Block: hotstuff.NewBlock(
				qc.BlockHash(),
				qc,
				cmd,
				cs.synchronizer.View(),
				cs.opts.ID(),
			),
		}

		if aggQC, ok := cert.AggQC(); ok && cs.opts.ShouldUseAggQC() {
			proposal.AggregateQC = &aggQC
		}
	}

	cs.blockChain.Store(proposal.Block)
	// kauri sends the proposal to only the children
	if cs.kauri == nil {
		cs.configuration.Propose(proposal)
	}
	// self vote
	cs.OnPropose(proposal)
}

// OnPropose handles incoming proposals (same as base implementation)
func (cs *persistentConsensusBase) OnPropose(proposal hotstuff.ProposeMsg) { //nolint:gocyclo
	// TODO: extract parts of this method into helper functions maybe?
	cs.logger.Debugf("OnPropose: %v", proposal.Block)

	block := proposal.Block

	if cs.opts.ShouldUseAggQC() && proposal.AggregateQC != nil {
		highQC, ok := cs.crypto.VerifyAggregateQC(*proposal.AggregateQC)
		if !ok {
			cs.logger.Warn("OnPropose: failed to verify aggregate QC")
			return
		}
		// NOTE: for simplicity, we require that the highQC found in the AggregateQC equals the QC embedded in the block.
		if !block.QuorumCert().Equals(highQC) {
			cs.logger.Warn("OnPropose: block QC does not equal highQC")
			return
		}
	}

	if !cs.crypto.VerifyQuorumCert(block.QuorumCert()) {
		cs.logger.Info("OnPropose: invalid QC")
		return
	}

	// ensure the block came from the leader.
	if proposal.ID != cs.leaderRotation.GetLeader(block.View()) {
		cs.logger.Info("OnPropose: block was not proposed by the expected leader")
		return
	}

	if !cs.impl.VoteRule(proposal) {
		cs.logger.Info("OnPropose: Block not voted for")
		return
	}

	if qcBlock, ok := cs.blockChain.Get(block.QuorumCert().BlockHash()); ok {
		cs.acceptor.Proposed(qcBlock.Command())
	} else {
		cs.logger.Info("OnPropose: Failed to fetch qcBlock")
	}

	if !cs.acceptor.Accept(block.Command()) {
		cs.logger.Info("OnPropose: command not accepted")
		return
	}

	// block is safe and was accepted
	cs.blockChain.Store(block)

	if b := cs.impl.CommitRule(block); b != nil {
		cs.commit(b)
	}
	cs.synchronizer.AdvanceView(hotstuff.NewSyncInfo().WithQC(block.QuorumCert()))

	// Check last vote with persistence
	cs.mut.Lock()
	shouldVote := block.View() > cs.lastVote
	if shouldVote {
		cs.lastVote = block.View()
	}
	cs.mut.Unlock()

	if !shouldVote {
		cs.logger.Info("OnPropose: block view too old")
		return
	}

	// Persist last vote change
	if err := cs.stateStore.SetLastVote(block.View()); err != nil {
		cs.logger.Errorf("Failed to persist last vote: %v", err)
	}

	pc, err := cs.crypto.CreatePartialCert(block)
	if err != nil {
		cs.logger.Error("OnPropose: failed to sign block: ", err)
		return
	}

	if cs.kauri != nil {
		cs.kauri.Begin(pc, proposal)
		return
	}
	leaderID := cs.leaderRotation.GetLeader(cs.lastVote + 1)
	if leaderID == cs.opts.ID() {
		cs.eventLoop.AddEvent(hotstuff.VoteMsg{ID: cs.opts.ID(), PartialCert: pc})
		return
	}

	leader, ok := cs.configuration.Replica(leaderID)
	if !ok {
		cs.logger.Warnf("Replica with ID %d was not found!", leaderID)
		return
	}

	leader.Vote(pc)
}

// commit handles block commitment with persistence
func (cs *persistentConsensusBase) commit(block *hotstuff.Block) {
	cs.mut.Lock()
	// can't recurse due to requiring the mutex, so we use a helper instead.
	err := cs.commitInner(block)
	cs.mut.Unlock()

	if err != nil {
		cs.logger.Warnf("failed to commit: %v", err)
		return
	}

	// Persist the committed block
	if err := cs.stateStore.SetCommittedBlockHash(block.Hash()); err != nil {
		cs.logger.Errorf("Failed to persist committed block: %v", err)
	}

	// prune the blockchain and handle forked blocks
	forkedBlocks := cs.blockChain.PruneToHeight(block.View())
	for _, block := range forkedBlocks {
		cs.forkHandler.Fork(block)
	}
}

// commitInner is a recursive helper for commit (same as base implementation)
func (cs *persistentConsensusBase) commitInner(block *hotstuff.Block) error {
	if cs.bExec.View() >= block.View() {
		return nil
	}
	if parent, ok := cs.blockChain.Get(block.Parent()); ok {
		err := cs.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}
	cs.eventLoop.AddEvent(hotstuff.ConsensusLatencyEvent{Latency: time.Since(block.Timestamp())})
	cs.logger.Debug("EXEC: ", block)
	cs.executor.Exec(block)
	cs.bExec = block
	return nil
}

// ChainLength returns the number of blocks that need to be chained together in order to commit
func (cs *persistentConsensusBase) ChainLength() int {
	return cs.impl.ChainLength()
}

// Close cleanly shuts down the persistent consensus
func (cs *persistentConsensusBase) Close() error {
	if cs.stateStore != nil {
		return cs.stateStore.Close()
	}
	return nil
}

var _ modules.Consensus = (*persistentConsensusBase)(nil)
