```
=========================================================================================
________________________________/\\\_____________________________________________________ 
________________________________\/\\\___________/\\\_____________________________________ 
______________________/\\\_______\/\\\__________\/\\\___________/\\\______/\\\___________ 
________/\\\\\\\\___/\\\\\\\\\\\__\/\\\__________\/\\\___ /\\___\///____/\\\\\\\\\\\_____ 
_______/\\\         \////\\\////___\/\\\\\\\\\\___\/\\\_ /\\\___________\////\\\////_____ 
_______/\\\\\\\\\\\_____\/\\\_______\/\\\/////\\\__\/\\\\\\_______\/\\\_____\/\\\________ 
_______\//\\\            \/\\\_______\/\\\___\/\\\__\/\\\__\/\\\___\/\\\_____\/\\\_______
________\//\\\\\\\\\\_____\//\\\\\____\/\\\___\/\\\__\/\\\__\/\\\___\/\\\_____\//\\\\\___ 
_________\///////////______\//////_____\///____\///___\///___\///____\///______\/////____

==================================== we <3 Ethereum =====================================
```

Ethkit is an Ethereum [CLI](#ethkit-cli) and [Go development kit](#ethkit-go-development-library)
designed to make it easier to use and develop for Ethereum.


## Ethkit CLI

Ethkit comes equipt with the `ethkit` CLI providing:
  * Wallet -- manage Ethereum wallets & accounts. restore wallets from a secret mnemonic.
    with scrypt wallet encryption support.
  * Abigen -- generate Go code from an ABI artifact file to interact with or deploy a smart
    contract.
  * Artifacts -- parse details from a Truffle artifact file from command line such as contract
    bytecode or the json abi


#### Install

```go get github.com/0xsequence/ethkit/cmd/ethkit```

#### wallet
```wallet``` handles encrypted Ethereum wallet creation and management in user-supplied keyfiles.
It allows users to create a new Ethereum wallet, import an existing Ethereum wallet from a secret
mnemonic or print an existing wallet's secret mnemonic.

```
Usage:
  ethkit wallet [flags]

Flags:
  -h, --help              help for wallet
      --import-mnemonic   import a secret mnemonic to a new keyfile
      --keyfile string    wallet key file path
      --new               create a new wallet and save it to the keyfile
      --print-account     print wallet account address from keyfile (default) (default true)
      --print-mnemonic    print wallet secret mnemonic from keyfile (danger!)
```


#### abigen
```abigen``` generates Go contract client code from a JSON [truffle](https://www.trufflesuite.com/)
artifacts file.

```Usage:
  ethkit abigen [flags]

Flags:
      --abiFile string         path to abi json file
      --artifactsFile string   path to truffle contract artifacts file
  -h, --help                   help for abigen
      --lang string            target language, supported: [go], default=go
      --outFile string         outFile (optional), default=stdout
      --pkg string             pkg (optional)
      --type string            type (optional)
```

#### artifacts
```artifacts``` prints the contract ABI or bytecode from a user-supplied truffle artifacts file.

```
Usage:
  ethkit artifacts [flags]

Flags:
      --abi           abi
      --bytecode      bytecode
      --file string   path to truffle contract artifacts file (required)
  -h, --help          help for artifacts
```


## Ethkit Go Development Library

Ethkit is a very capable Ethereum development library for writing systems in Go that
interface with an Ethereum-compatible network (mainnet/testnet or another EVM sidechain).
We use it in the Sequence stack for many micro-services in our infrastructure,
we hope you enjoy it too.

Packages:

* `ethartifacts`: simple pkg to parse Truffle artifact file
* `ethcoder`: encoding/decoding libraries for smart contracts and transactions
* `ethdeploy`: simple method to deploy contract bytecode to a network
* `ethgas`: fetch the latest gas price of a network or track over a period of time
* `ethmonitor`: easily monitor block production, transactions and logs of a chain; with re-org support
* `ethrpc`: http client for Ethereum json-rpc
* `ethwallet`: wallet for Ethereum with support for wallet mnemonics (BIP-39)


## License

Copyright (c) 2018-present [Horizon Blockchain Games Inc.](https://horizon.io)

Licensed under [MIT License](./LICENSE)

[GoDoc]: https://pkg.go.dev/github.com/0xsequence/ethkit
[GoDoc Widget]: https://godoc.org/github.com/0xsequence/ethkit?status.svg
