package synchronizer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

// PersistentSynchronizer synchronizes replicas to the same view with persistent state
type PersistentSynchronizer struct {
	blockChain     modules.BlockChain
	consensus      modules.Consensus
	crypto         modules.Crypto
	configuration  modules.Configuration
	eventLoop      *eventloop.EventLoop
	leaderRotation modules.LeaderRotation
	logger         logging.Logger
	opts           *modules.Options

	// Persistent state store
	stateStore *blockchain.StateStore

	mut         sync.RWMutex // to protect the following
	currentView hotstuff.View
	highTC      hotstuff.TimeoutCert
	highQC      hotstuff.QuorumCert

	// A pointer to the last timeout message that we sent.
	// If a timeout happens again before we advance to the next view,
	// we will simply send this timeout again.
	lastTimeout *hotstuff.TimeoutMsg

	duration ViewDuration
	timer    oneShotTimer

	// map of collected timeout messages per view (not persisted - runtime only)
	timeouts map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg
}

// NewPersistent creates a new PersistentSynchronizer
func NewPersistent(viewDuration ViewDuration, dataDir string) (modules.Synchronizer, error) {
	stateStore, err := blockchain.NewStateStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create state store: %w", err)
	}

	return NewPersistentWithStateStore(viewDuration, stateStore)
}

// NewPersistentWithStateStore creates a new PersistentSynchronizer with an existing state store
func NewPersistentWithStateStore(viewDuration ViewDuration, stateStore *blockchain.StateStore) (modules.Synchronizer, error) {
	return &PersistentSynchronizer{
		stateStore:  stateStore,
		currentView: 1, // will be loaded from state store in InitModule

		duration: viewDuration,
		timer:    oneShotTimer{time.AfterFunc(0, func() {})}, // dummy timer that will be replaced after start() is called

		timeouts: make(map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg),
	}, nil
}

// InitModule initializes the persistent synchronizer
func (s *PersistentSynchronizer) InitModule(mods *modules.Core) {
	mods.Get(
		&s.blockChain,
		&s.consensus,
		&s.crypto,
		&s.configuration,
		&s.eventLoop,
		&s.leaderRotation,
		&s.logger,
		&s.opts,
	)

	s.eventLoop.RegisterHandler(TimeoutEvent{}, func(event any) {
		timeoutView := event.(TimeoutEvent).View
		if s.View() == timeoutView {
			s.OnLocalTimeout()
		}
	})

	s.eventLoop.RegisterHandler(hotstuff.NewViewMsg{}, func(event any) {
		newViewMsg := event.(hotstuff.NewViewMsg)
		s.OnNewView(newViewMsg)
	})

	s.eventLoop.RegisterHandler(hotstuff.TimeoutMsg{}, func(event any) {
		timeoutMsg := event.(hotstuff.TimeoutMsg)
		s.OnRemoteTimeout(timeoutMsg)
	})

	// Load persistent state
	if err := s.loadState(); err != nil {
		s.logger.Errorf("Failed to load persistent synchronizer state: %v", err)
		// Continue with defaults if loading fails
		s.initializeDefaults()
	}
}

// loadState loads the synchronizer state from persistent storage
func (s *PersistentSynchronizer) loadState() error {
	s.mut.Lock()
	defer s.mut.Unlock()

	// Load current view
	currentView, err := s.stateStore.GetCurrentView()
	if err != nil {
		return fmt.Errorf("failed to load current view: %w", err)
	}
	s.currentView = currentView

	// Load high QC
	highQC, err := s.stateStore.GetHighQC()
	if err != nil {
		return fmt.Errorf("failed to load high QC: %w", err)
	}
	s.highQC = highQC

	// Load high TC
	highTC, err := s.stateStore.GetHighTC()
	if err != nil {
		return fmt.Errorf("failed to load high TC: %w", err)
	}
	s.highTC = highTC

	s.logger.Infof("Loaded persistent synchronizer state: view=%d, highQC.view=%d, highTC.view=%d",
		s.currentView, s.highQC.View(), s.highTC.View())

	return nil
}

// initializeDefaults initializes default state when loading fails
func (s *PersistentSynchronizer) initializeDefaults() {
	s.mut.Lock()
	defer s.mut.Unlock()

	var err error
	s.highQC, err = s.crypto.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
	s.highTC, err = s.crypto.CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
	}

	s.currentView = 1

	// Persist the defaults
	s.persistState()
}

// persistState saves the synchronizer state to persistent storage
func (s *PersistentSynchronizer) persistState() error {
	updates := map[string]interface{}{
		"current_view": s.currentView,
	}

	if err := s.stateStore.UpdateConsensusState(updates); err != nil {
		return err
	}

	if err := s.stateStore.SetHighQC(s.highQC); err != nil {
		return err
	}

	if err := s.stateStore.SetHighTC(s.highTC); err != nil {
		return err
	}

	return nil
}

// Start starts the synchronizer with the given context
func (s *PersistentSynchronizer) Start(ctx context.Context) {
	s.startTimeoutTimer()

	go func() {
		<-ctx.Done()
		s.stopTimeoutTimer()
	}()

	// start the initial proposal
	if view := s.View(); view == 1 && s.leaderRotation.GetLeader(view) == s.opts.ID() {
		s.consensus.Propose(s.SyncInfo())
	}
}

// HighQC returns the highest known QC
func (s *PersistentSynchronizer) HighQC() hotstuff.QuorumCert {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.highQC
}

// View returns the current view
func (s *PersistentSynchronizer) View() hotstuff.View {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.currentView
}

// SyncInfo returns the highest known QC or TC
func (s *PersistentSynchronizer) SyncInfo() hotstuff.SyncInfo {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return hotstuff.NewSyncInfo().WithQC(s.highQC).WithTC(s.highTC)
}

// OnLocalTimeout is called when a local timeout happens
func (s *PersistentSynchronizer) OnLocalTimeout() {
	s.startTimeoutTimer()

	view := s.View()

	if s.lastTimeout != nil && s.lastTimeout.View == view {
		s.configuration.Timeout(*s.lastTimeout)
		return
	}

	s.duration.ViewTimeout() // increase the duration of the next view
	s.logger.Debugf("OnLocalTimeout: %v", view)

	sig, err := s.crypto.Sign(view.ToBytes())
	if err != nil {
		s.logger.Warnf("Failed to sign view: %v", err)
		return
	}
	timeoutMsg := hotstuff.TimeoutMsg{
		ID:            s.opts.ID(),
		View:          view,
		SyncInfo:      s.SyncInfo(),
		ViewSignature: sig,
	}

	if s.opts.ShouldUseAggQC() {
		// generate a second signature that will become part of the aggregateQC
		sig, err := s.crypto.Sign(timeoutMsg.ToBytes())
		if err != nil {
			s.logger.Warnf("Failed to sign timeout message: %v", err)
			return
		}
		timeoutMsg.MsgSignature = sig
	}
	s.lastTimeout = &timeoutMsg
	// stop voting for current view
	s.consensus.StopVoting(view)

	s.configuration.Timeout(timeoutMsg)
	s.OnRemoteTimeout(timeoutMsg)
}

// OnRemoteTimeout handles an incoming timeout from a remote replica
func (s *PersistentSynchronizer) OnRemoteTimeout(timeout hotstuff.TimeoutMsg) {
	currView := s.View()

	defer func() {
		// cleanup old timeouts
		for view := range s.timeouts {
			if view < currView {
				delete(s.timeouts, view)
			}
		}
	}()

	verifier := s.crypto
	if !verifier.Verify(timeout.ViewSignature, timeout.View.ToBytes()) {
		return
	}
	s.logger.Debug("OnRemoteTimeout: ", timeout)

	s.AdvanceView(timeout.SyncInfo)

	timeouts, ok := s.timeouts[timeout.View]
	if !ok {
		timeouts = make(map[hotstuff.ID]hotstuff.TimeoutMsg)
		s.timeouts[timeout.View] = timeouts
	}

	if _, ok := timeouts[timeout.ID]; !ok {
		timeouts[timeout.ID] = timeout
	}

	if len(timeouts) < s.configuration.QuorumSize() {
		return
	}

	// TODO: should probably change CreateTimeoutCert and maybe also CreateQuorumCert
	// to use maps instead of slices
	timeoutList := make([]hotstuff.TimeoutMsg, 0, len(timeouts))
	for _, t := range timeouts {
		timeoutList = append(timeoutList, t)
	}

	tc, err := s.crypto.CreateTimeoutCert(timeout.View, timeoutList)
	if err != nil {
		s.logger.Debugf("Failed to create timeout certificate: %v", err)
		return
	}

	si := s.SyncInfo().WithTC(tc)

	if s.opts.ShouldUseAggQC() {
		aggQC, err := s.crypto.CreateAggregateQC(currView, timeoutList)
		if err != nil {
			s.logger.Debugf("Failed to create aggregateQC: %v", err)
		} else {
			si = si.WithAggQC(aggQC)
		}
	}

	delete(s.timeouts, timeout.View)

	s.AdvanceView(si)
}

// OnNewView handles an incoming consensus.NewViewMsg
func (s *PersistentSynchronizer) OnNewView(newView hotstuff.NewViewMsg) {
	s.AdvanceView(newView.SyncInfo)
}

// AdvanceView attempts to advance to the next view using the given QC with persistence
func (s *PersistentSynchronizer) AdvanceView(syncInfo hotstuff.SyncInfo) {
	v := hotstuff.View(0)
	timeout := false

	// check for a TC
	if tc, ok := syncInfo.TC(); ok {
		if !s.crypto.VerifyTimeoutCert(tc) {
			s.logger.Info("Timeout Certificate could not be verified!")
			return
		}
		s.updateHighTC(tc)
		v = tc.View()
		timeout = true
	}

	var (
		haveQC bool
		qc     hotstuff.QuorumCert
		aggQC  hotstuff.AggregateQC
	)

	// check for an AggQC or QC
	if aggQC, haveQC = syncInfo.AggQC(); haveQC && s.opts.ShouldUseAggQC() {
		highQC, ok := s.crypto.VerifyAggregateQC(aggQC)
		if !ok {
			s.logger.Info("Aggregated Quorum Certificate could not be verified")
			return
		}
		if aggQC.View() >= v {
			v = aggQC.View()
			timeout = true
		}
		// ensure that the true highQC is the one stored in the syncInfo
		syncInfo = syncInfo.WithQC(highQC)
		qc = highQC
	} else if qc, haveQC = syncInfo.QC(); haveQC {
		if !s.crypto.VerifyQuorumCert(qc) {
			s.logger.Info("Quorum Certificate could not be verified!")
			return
		}
	}

	if haveQC {
		s.updateHighQC(qc)
		// if there is both a TC and a QC, we use the QC if its view is greater or equal to the TC.
		if qc.View() >= v {
			v = qc.View()
			timeout = false
		}
	}

	if v < s.View() {
		return
	}

	s.stopTimeoutTimer()

	if !timeout {
		s.duration.ViewSucceeded()
	}

	newView := v + 1

	// Update state with persistence
	s.mut.Lock()
	s.currentView = newView
	s.lastTimeout = nil
	persistNeeded := true
	s.mut.Unlock()

	// Persist the view change
	if persistNeeded {
		if err := s.stateStore.SetCurrentView(newView); err != nil {
			s.logger.Errorf("Failed to persist current view: %v", err)
		}
	}

	s.duration.ViewStarted()
	s.startTimeoutTimer()

	s.logger.Debugf("advanced to view %d", newView)
	s.eventLoop.AddEvent(ViewChangeEvent{View: newView, Timeout: timeout})

	leader := s.leaderRotation.GetLeader(newView)
	if leader == s.opts.ID() {
		s.consensus.Propose(syncInfo)
	} else if replica, ok := s.configuration.Replica(leader); ok {
		replica.NewView(syncInfo)
	}
}

// updateHighQC attempts to update the highQC with persistence
func (s *PersistentSynchronizer) updateHighQC(qc hotstuff.QuorumCert) {
	newBlock, ok := s.blockChain.Get(qc.BlockHash())
	if !ok {
		s.logger.Info("updateHighQC: Could not find block referenced by new QC!")
		return
	}

	s.mut.Lock()
	updated := false
	if newBlock.View() > s.highQC.View() {
		s.highQC = qc
		updated = true
		s.logger.Debug("HighQC updated")
	}
	s.mut.Unlock()

	// Persist the change if updated
	if updated {
		if err := s.stateStore.SetHighQC(qc); err != nil {
			s.logger.Errorf("Failed to persist high QC: %v", err)
		}
	}
}

// updateHighTC attempts to update the highTC with persistence
func (s *PersistentSynchronizer) updateHighTC(tc hotstuff.TimeoutCert) {
	s.mut.Lock()
	updated := false
	if tc.View() > s.highTC.View() {
		s.highTC = tc
		updated = true
		s.logger.Debug("HighTC updated")
	}
	s.mut.Unlock()

	// Persist the change if updated
	if updated {
		if err := s.stateStore.SetHighTC(tc); err != nil {
			s.logger.Errorf("Failed to persist high TC: %v", err)
		}
	}
}

// Timer management (same as original implementation)

func (s *PersistentSynchronizer) startTimeoutTimer() {
	view := s.View()
	s.timer = oneShotTimer{time.AfterFunc(s.duration.Duration(), func() {
		s.eventLoop.AddEvent(TimeoutEvent{view})
	})}
}

func (s *PersistentSynchronizer) stopTimeoutTimer() {
	s.timer.Stop()
}

// Close cleanly shuts down the persistent synchronizer
func (s *PersistentSynchronizer) Close() error {
	if s.stateStore != nil {
		return s.stateStore.Close()
	}
	return nil
}

var _ modules.Synchronizer = (*PersistentSynchronizer)(nil)
