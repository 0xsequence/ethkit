ethreceipts TODO
================

- [x] receipt.Filter = filter .. + <- -- copy before sending? lets write a test
- [x] erc20 blast + log + event test
- [x] pastReceipts cache
- [x] review locks..? always test with -race
- [x] FetchTransactionReceipt method..
- [x] finalizer -- what if there is a reorg? we should be reporting this.. is that the case?
- [x] MaxWait stuff..
- [x] SearchOnChain vs SearchCache ..?
- [x] notFound .. ethmonitor
- [x] downgrade to cachestore v0.5
- [ ] receipt.go
- [x] types core ... From, etc
- [ ] getBlockNumber from provider..
- [ ] better error types..
- [ ] final review..

- [ ] try this on go-sequence ReceiptListener -- update there, and run tests again, etc.
  - [ ] add helper method to filter for FilterMetaTxnID() ..
- [ ] Jakub, what kind of filter..? 
  - [ ] Search for a specific txnhash
  - [ ] Search for a specific event log
  - [ ] Filter events from a specific contract address