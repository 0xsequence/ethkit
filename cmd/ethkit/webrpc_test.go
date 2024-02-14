package main

import "testing"

func TestWebrpc(t *testing.T) {
	rpc := &Webrpc{}
	rpc.Run(nil, []string{})
}
