// see https://hardhat.org/hardhat-network/docs/reference#config
module.exports = {
  solidity: "0.8.28",

  networks: {
    hardhat: {
      mining: {
        auto: false,
        interval: 1000
      },

      // gas: 10000000000000,
      // blockGasLimit: 10000000000000,
      // gasPrice: 2,
      initialBaseFeePerGas: 1,
      chainId: 1337,
      accounts: {
        mnemonic: 'major danger this key only test please avoid main net use okay'
      },
      // loggingEnabled: true
      // verbose: true
    },
  }
}
