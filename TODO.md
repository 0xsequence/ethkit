ethreceipts TODO
================

- [x] receipt.Filter = filter .. + <- -- copy before sending? lets write a test
- [x] erc20 blast + log + event test
- [x] pastReceipts cache
- [~] review locks..? always test with -race
- [x] FetchTransactionReceipt method..
- [x] finalizer -- what if there is a reorg? we should be reporting this.. is that the case?
- [x] MaxWait stuff..
- [ ] SearchOnChain vs SearchCache ..?

- [ ] try this on go-sequence ReceiptListener -- update there, and run tests again, etc.
- [ ] Jakub, what kind of filter..? 
  - [ ] Search for a specific txnhash
  - [ ] Search for a specific event log
  - [ ] Filter events from a specific contract address