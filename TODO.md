TODO
====

ethtxn and ethwallet improvements .. naming etc.

------

1. Look at some of the viem and ox terminology.. "provider" ..?
2. rename..? ethrpc.Client and ethrpc.Provider .. and .NewClient .. its a bit confusing
    .... lets ask the AI its opinion..


------



1. ethreceipts using AsMessage .. dont need to, see note in receipt.go
2. search "TODO" in ethkit..

---


4. ethrpc, pass in a json marshaller, would be better.. 
5. review ethmonitor for the logs and streaming stuff, to build blocks from
subscriptions, etc.

6. any other cleanup in ethkit I should do...? lets make a list anyways..





--------

IMPROVEMENTS
============

1. ethmonitor: newHeads and newLogs .. can we use these..?
2. ethreceipts: review TODO's to make it more efficient -- not worth it right now..
3. ethrpc: remove pkg-level methods and pass in optional jsonmarshaller/unmarshaller -- have an idea how to do it..
4. ethrpc: rename interface.go -- yes, at the right time..


---------

ethmonitor..

1. lets adapt chain-newheads to see how "newHeads" looks..?