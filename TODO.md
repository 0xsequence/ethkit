ethreceipts TODO
================

- [ ] receipt.Filter = filter .. + <- -- copy before sending? lets write a test
- [ ] erc20 blast + log + event test
- [ ] pastReceipts cache
- [ ] review locks..? always test with -race
- [ ] FetchTransactionReceipt method..
- [ ] finalizer -- what if there is a reorg? we should be reporting this.. is that the case?
- [ ] MaxNumBlocksListen stuff..
- [ ] SearchHistory vs SearchCache ..?

- [ ] try this on go-sequence ReceiptListener -- update there, and run tests again, etc.