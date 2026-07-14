package statesync

import "github.com/cosmos/gogoproto/proto"

// ValidateMessage exposes the ordinary pure State Sync message validation for
// controller callback admission without changing Reactor behavior.
func ValidateMessage(message proto.Message) error {
	return validateMsg(message)
}
