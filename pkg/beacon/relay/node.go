package relay

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"sync"

	relaychain "github.com/keep-network/keep-core/pkg/beacon/relay/chain"
	"github.com/keep-network/keep-core/pkg/beacon/relay/config"
	"github.com/keep-network/keep-core/pkg/beacon/relay/dkg2"
	"github.com/keep-network/keep-core/pkg/beacon/relay/groupselection"
	"github.com/keep-network/keep-core/pkg/chain"
	"github.com/keep-network/keep-core/pkg/net"
)

// Node represents the current state of a relay node.
type Node struct {
	mutex sync.Mutex

	// Staker is an on-chain identity that this node is using to prove its
	// stake in the system.
	Staker chain.Staker

	// External interactors.
	netProvider  net.Provider
	blockCounter chain.BlockCounter
	chainConfig  *config.Chain

	// The IDs of the known stakes in the system, including this node's StakeID.
	stakeIDs      []string
	maxStakeIndex int

	groupPublicKeys [][]byte
	seenPublicKeys  map[string]struct{}
	myGroups        map[string]*membership
	pendingGroups   map[string]*membership
}

type membership struct {
	member  *dkg2.ThresholdSigner
	channel net.BroadcastChannel
	index   int
}

// JoinGroupIfEligible takes a threshold relay entry value and undergoes the
// process of joining a group if this node's virtual stakers prove eligible for
// the group generated by that entry. This is an interactive on-chain process,
// and JoinGroupIfEligible can block for an extended period of time while it
// completes the on-chain operation.
//
// Indirectly, the completion of the process is signaled by the formation of an
// on-chain group containing at least one of this node's virtual stakers.
func (n *Node) JoinGroupIfEligible(
	relayChain relaychain.Interface,
	groupSelectionResult *groupselection.Result,
	entryRequestID *big.Int,
	entrySeed *big.Int,
) {
	// build the channel name and get the broadcast channel
	broadcastChannelName := channelNameFromSelectedTickets(
		groupSelectionResult.SelectedTickets,
	)
	broadcastChannel, err := n.netProvider.ChannelFor(
		broadcastChannelName,
	)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Failed to get broadcastChannel for name %s with err: [%v].\n",
			broadcastChannelName,
			err,
		)
		return
	}

	for index, ticket := range groupSelectionResult.SelectedTickets {
		// If our ticket is amongst those chosen, kick
		// off an instance of DKG. We may have multiple
		// tickets in the selected tickets (which would
		// result in multiple instances of DKG).
		if ticket.IsFromStaker(n.Staker.ID()) {
			go dkg2.ExecuteDKG(
				entryRequestID,
				entrySeed,
				index,
				n.chainConfig.GroupSize,
				n.chainConfig.Threshold,
				n.blockCounter,
				relayChain,
				broadcastChannel,
			)
		}
	}
	// exit on signal
	return
}

// channelNameFromSelectedTickets takes the selected tickets, and does the
// following to construct the broadcastChannel name:
// * grabs the value from each ticket
// * concatenates all of the values
// * returns the hashed concatenated values
func channelNameFromSelectedTickets(
	tickets []*groupselection.Ticket,
) string {
	var channelNameBytes []byte
	for _, ticket := range tickets {
		channelNameBytes = append(
			channelNameBytes,
			ticket.Value.Bytes()...,
		)
	}
	hashedChannelName := groupselection.SHAValue(
		sha256.Sum256(channelNameBytes),
	)
	return string(hashedChannelName.Bytes())
}

// RegisterGroup registers that a group was successfully created by the given
// requestID, and its group public key is groupPublicKey.
func (n *Node) RegisterGroup(requestID string, groupPublicKey []byte) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	// If we've already registered a group for this request ID, return early.
	if _, exists := n.seenPublicKeys[requestID]; exists {
		return
	}

	n.seenPublicKeys[requestID] = struct{}{}
	n.groupPublicKeys = append(n.groupPublicKeys, groupPublicKey)
	index := len(n.groupPublicKeys) - 1

	if membership, found := n.pendingGroups[requestID]; found && membership != nil {
		membership.index = index
		n.myGroups[requestID] = membership
		delete(n.pendingGroups, requestID)
	}
}

// initializePendingGroup grabs ownership of an attempt at group creation for a
// given goroutine. If it returns false, we're already in progress and failed to
// initialize.
func (n *Node) initializePendingGroup(requestID string) bool {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	// If the pending group exists, we're already active
	if _, found := n.pendingGroups[requestID]; found {
		return false
	}

	// Pending group does not exist, take control
	n.pendingGroups[requestID] = nil

	return true
}

// flushPendingGroup if group creation fails, we clean our references to creating
// a group for a given request ID.
func (n *Node) flushPendingGroup(requestID string) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if membership, found := n.pendingGroups[requestID]; found && membership == nil {
		delete(n.pendingGroups, requestID)
	}
}

// registerPendingGroup assigns a new membership for a given request ID.
// We overwrite our placeholder membership set by initializePendingGroup.
func (n *Node) registerPendingGroup(
	requestID string,
	signer *dkg2.ThresholdSigner,
	channel net.BroadcastChannel,
) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if _, seen := n.seenPublicKeys[requestID]; seen {
		groupPublicKey := signer.GroupPublicKeyBytes()
		// Start at the end since it's likely the public key was closer to the
		// end if it happened to come in before we had a chance to register it
		// as pending.
		existingIndex := len(n.groupPublicKeys) - 1
		for ; existingIndex >= 0; existingIndex-- {
			if reflect.DeepEqual(n.groupPublicKeys[existingIndex], groupPublicKey[:]) {
				break
			}
		}

		n.myGroups[requestID] = &membership{
			index:   existingIndex,
			member:  signer,
			channel: channel,
		}
		delete(n.pendingGroups, requestID)
	} else {
		n.pendingGroups[requestID] = &membership{
			member:  signer,
			channel: channel,
		}
	}
}
