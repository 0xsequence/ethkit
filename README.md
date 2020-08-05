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

Ethkit is an [Ethereum](https://ethereum.org/) wallet and toolkit meant to make it easer to work with Ethereum.
It's has 3 components: ```abigen```, ```wallet``` and ```artifacts```.
It allows users to manage Ethereum wallets, restore wallets from a secret mnemonic and display their secret mnemonic.

### Subcommands

#### abigen
```abigen``` generates contract client code from a JSON truffle artifacts file.

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

#### wallet
```wallet``` handles encrypted Ethereum wallet creation and management in user-supplied keyfiles.
It allows users to create a new Ethereum wallet, import an existing Ethereum wallet from a secret mnemonic or print an existing wallet's secret mnemonic.

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

### Building Ethkit
Ethkit is written in Go and can be built simply using ```go build```.
To make your life easier, we've included a Makefile.
You can build and install the ethkit CLI to ```$GOPATH/bin``` using ```make install```.

### Running the tests
You can run Ethkit's test suite with ```make test```.

### Upgrading dependencies
To upgrade the dependencies, run ```make dep-upgrade-all```.