The receipt in `mainnet-withdrawal-receipt.json` was fetched with the read-only
`eth_getTransactionReceipt` RPC from `https://opbnb-mainnet-rpc.bnbchain.org`.

- Network: opBNB mainnet, chain ID 204
- Block: 13113334
- Transaction: `0x77cc9c7392442c289d30112032203e387e0fe04273a6eafe5863430aa5900c07`
- Helper: `0x4000698e3De52120DE28181BaACda82B21568416`

The fixture verifies the deployed helper event ABI and reconstructs the portal
withdrawal against the emitted `MessagePassed` hash, entirely offline.
