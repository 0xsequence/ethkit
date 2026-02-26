package ethrpc

import (
	"encoding/json"
	"fmt"

	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
)

type BatchCall []*Call

func (b *BatchCall) MarshalJSON() ([]byte, error) {
	if len(*b) == 1 {
		return json.Marshal((*b)[0].request)
	}
	reqBody := make([]jsonrpc.Message, len(*b))
	for i, r := range *b {
		reqBody[i] = r.request
	}
	return json.Marshal(reqBody)
}

func (b *BatchCall) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("failed to unmarshal batch response: empty body")
	}

	var results []*jsonrpc.Message
	if data[0] != '[' {
		results = make([]*jsonrpc.Message, 1)
		if err := json.Unmarshal(data, &results[0]); err != nil {
			return fmt.Errorf("failed to unmarshal batch response: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &results); err != nil {
			return fmt.Errorf("failed to unmarshal batch response: %w", err)
		}
	}
	if len(results) == 0 {
		return fmt.Errorf("failed to unmarshal batch response: empty result set")
	}

	callByID := make(map[uint64]*Call, len(*b))
	for i, call := range *b {
		if call == nil {
			return fmt.Errorf("nil call at index %d", i)
		}
		id := call.request.ID
		if _, exists := callByID[id]; exists {
			return fmt.Errorf("duplicate request id %d", id)
		}
		callByID[id] = call
	}

	for i, msg := range results {
		if msg == nil {
			return fmt.Errorf("nil response at index %d", i)
		}
		call, ok := callByID[msg.ID]
		if !ok {
			return fmt.Errorf("response id %d does not match any request", msg.ID)
		}
		if call.response != nil {
			return fmt.Errorf("duplicate response for id %d", msg.ID)
		}
		call.response = msg
		if msg.Error != nil {
			call.err = *msg.Error
		}
	}
	return nil
}

func (b *BatchCall) ErrorOrNil() error {
	err := make(BatchError)
	for i, r := range *b {
		if r.err != nil {
			err[i] = r
		}
	}
	if len(err) > 0 {
		return err
	}
	return nil
}

type BatchError map[int]*Call

func (e BatchError) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e.Unwrap().Error()
	}
	return fmt.Sprintf("%d errors", len(e))
}

func (e BatchError) ErrorMap() map[int]error {
	errMap := make(map[int]error, len(e))
	for i, c := range e {
		errMap[i] = c
	}
	return errMap
}

func (e BatchError) Unwrap() error {
	for _, nested := range e {
		return nested
	}
	return nil
}
