# L2StandardBridgeBot on opBNB mainnet

The helper uses the opBNB mainnet L2StandardBridge predeploy at
`0x4200000000000000000000000000000000000010` (chain ID 204).
`WithdrawTo(address,address,address,uint256,uint32,bytes)` has indexed `from`
and `l2Token` arguments and matches the Go binding in `../bindings`.

From the repository root:

```bash
git submodule update --init --recursive
make build-solidity
```

The fork tests use mainnet state and execute transactions only inside the local fork:

```bash
OPBNB_MAINNET=https://opbnb-mainnet-rpc.bnbchain.org forge test --root contracts
```

See the [mainnet deployment and usage instructions](../README.md#deploy-and-use-contracts)
for commands to use later. Building and testing do not deploy or broadcast transactions.

Foundry documentation: https://book.getfoundry.sh/
