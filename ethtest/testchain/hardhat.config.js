// see https://hardhat.org/hardhat-network/docs/reference#config
export default {
  solidity: {
    version: "0.8.28",
    settings: {
      /* solc settings */
    }
  },

  networks: {
    hardhat: {
      type: "edr-simulated",
      chainType: "l1",

      chainId: 1337,

      mining: {
        auto: false,
        interval: 1000
      },

      // gas: 10000000000000,
      // blockGasLimit: 10000000000000,
      // gasPrice: 2,
      initialBaseFeePerGas: 1,
      
       accounts: {
        mnemonic: 'major danger this key only test please avoid main net use okay'
      },

      // loggingEnabled: true
      // verbose: true
    },
  },

  test: {
    solidity: {
      timeout: 40000,
      // other solidity tests options
    },
  },
}
