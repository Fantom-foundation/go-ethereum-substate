// Package ethclient provides a client for the Fantom RPC API.
package ethclient

import (
	"context"
	"encoding/json"

	"github.com/ethereum/go-ethereum"
)

// GetEventPayload returns Lachesis event by hash or short ID.
func (ec *Client) GetEventPayload(ctx context.Context, shortEventID string, inclTx bool) (raw json.RawMessage, err error) {
	// var raw json.RawMessage
	err = ec.c.CallContext(ctx, &raw, "dag_getEventPayload", shortEventID, inclTx)
	if err != nil {
		return nil, err
	} else if len(raw) == 0 {
		return nil, ethereum.NotFound
	}

	return
}
