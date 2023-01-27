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
	var (
		results []*jsonrpc.Message
		target  any = &results
	)
	if data[0] != '[' {
		results = make([]*jsonrpc.Message, 1)
		target = &results[0]
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal batch response: %w", err)
	}
	if len(results) > len(*b) {
		return fmt.Errorf("more responses (%d) than requests (%d)", len(results), len(*b))
	}
	for i, msg := range results {
		(*b)[i].response = msg
		if msg.Error != nil {
			(*b)[i].err = msg.Error
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
