# opbnb-bridge-bot

This bridge helper targets **opBNB mainnet and BSC mainnet**. The
[`L2StandardBridgeBot`](contracts/src/L2StandardBridgeBot.sol) contract collects
a delegation fee and forwards withdrawals to the L2StandardBridge. The Go bot
watches its `WithdrawTo` events, stores pending withdrawals in MySQL, and sends
the proof and finalization transactions to the L1 OptimismPortal.

A withdrawal has three steps:

1. Initiate the withdrawal on opBNB.
2. Prove it on BSC after the L2OutputOracle publishes an output covering its block.
3. Finalize it on BSC after the seven-day challenge period following the proof.

![Withdrawal flow](./assets/optimism-withdraw-flow.png)

## Mainnet configuration

[`bot.mainnet.toml`](bot.mainnet.toml) is the default configuration. Its protocol addresses
are confirmed by the official [protocol address documentation](https://docs.bnbchain.org/bnb-opbnb/core-concepts/opbnb-protocol-addresses/).

| Setting | Mainnet value |
| --- | --- |
| L1 chain ID / RPC | BSC **56** / `https://bsc-dataseed.bnbchain.org` |
| L2 chain ID / RPC | opBNB **204** / `https://opbnb-mainnet-rpc.bnbchain.org` |
| L2StandardBridge | `0x4200000000000000000000000000000000000010` |
| OptimismPortal proxy on BSC | `0x1876EA7702C0ad0C6A2ae6036DE7733edfBca519` |
| L2OutputOracle proxy on BSC | `0x153CAB79f4767E2ff862C94aa49573294B13D169` |
| L1CrossDomainMessenger proxy on BSC | `0xd95D508f13f7029CCF0fb61984d5dfD11b879c4f` |
| Existing upstream L2StandardBridgeBot on opBNB | `0x4000698e3De52120DE28181BaACda82B21568416` |
| Challenge window | **604800 seconds (7 days)** |

The helper address is taken from the repository's original mainnet configuration;
it is a separate contract from the L2StandardBridge predeploy. If you deploy your
own helper later, set `l2-standard-bridge-bot.contract-address` and
`l2-starting-number` to your deployment address and block.

The bot verifies both RPC chain IDs before connecting to MySQL. It reads the
signing key exclusively from `OPBNB_BRIDGE_BOT_PRIVKEY`, supports keys with or
without a `0x` prefix, and never logs the expanded configuration. RPC URLs can
be changed in a local `bot.toml`; the L2 provider must support historical blocks,
receipts, logs and `eth_getProof` for the oracle output block used to prove a
withdrawal. Backfilling the existing helper may require an archive provider.

The MySQL schema is unchanged: `l2_scanned_block_v2` tracks progress and
`withdrawal_initiated_log_v2` stores pending withdrawals, `proven_time`,
`finalized_time` and `failure_reason`. Use a dedicated mainnet database;
do not reuse a database containing withdrawals or scan progress from another
network or helper deployment. The included Compose file creates
`opbnb_bridge_bot_mainnet` in a separate `mysql_mainnet_data` volume.

The indexer checks the helper's event signature and both indexed arguments.
Withdrawal reconstruction uses the bridge and messenger events from the same
receipt and checks the result against the `MessagePassed` payload and withdrawal
hash before proving or finalizing. Finalization is selected only after the
configured seven-day period; the portal also enforces its on-chain timing.
The proposer setting is retained, but proof readiness is determined by the
oracle's published block, rather than a fixed wall-clock delay.

References: [network information](https://docs.bnbchain.org/bnb-opbnb/get-started/network-info/),
[seven-day withdrawal period](https://docs.bnbchain.org/bnb-opbnb/get-started/withdraw-from-opbnb/),
[mainnet bridge](https://opbnb-bridge.bnbchain.org),
[L2StandardBridge explorer](https://opbnbscan.com/address/0x4200000000000000000000000000000000000010),
[OptimismPortal explorer](https://bscscan.com/address/0x1876EA7702C0ad0C6A2ae6036DE7733edfBca519).

## Build and run on mainnet

Requirements: Go 1.21 or newer, Make, Foundry (`forge`), and MySQL 8.0.
The included database example also requires Docker Compose.
Run from the repository root:

```bash
git submodule update --init --recursive
make build-go
make build-solidity
cp bot.mainnet.toml bot.toml

# Non-secret connection settings matching the included local MySQL service.
set -a
source docker/mainnet-db.sh
set +a
docker compose -f docker/docker-compose.yml up -d mysql
```

Wait for MySQL to finish initializing, then load your bot key into the environment
using a secret manager or this hidden Bash prompt. The bot account needs BNB on
BSC to pay for proof and finalization gas.

```bash
read -rsp 'Bot private key: ' OPBNB_BRIDGE_BOT_PRIVKEY
export OPBNB_BRIDGE_BOT_PRIVKEY
OPBNB_BRIDGE_BOT_PRIVKEY="$OPBNB_BRIDGE_BOT_PRIVKEY" ./bot --config ./bot.toml
```

Starting the bot enables real mainnet proof and finalization transactions for
eligible withdrawals. Build and test commands alone do not send transactions.
The existing `./bot run --config ./bot.toml` form is also supported.
`./bot --help` prints usage without starting the bot.

For an existing MySQL installation, export `OPBNB_BRIDGE_BOT_DB_HOST`,
`OPBNB_BRIDGE_BOT_DB_PORT`, `OPBNB_BRIDGE_BOT_DB_USER` and
`OPBNB_BRIDGE_BOT_DB_PASSWORD`, and update `[db].name` in your local config.
Do not commit signing keys or secret files. Local `bot.toml` and `.env` files
are ignored by Git.

Run local checks:

```bash
go test ./...
OPBNB_MAINNET=https://opbnb-mainnet-rpc.bnbchain.org forge test --root contracts
```

## Deploy and use contracts

These commands are provided for a future mainnet deployment and withdrawal.
They send real transactions only when you run them; deployment is not required
to build the project or watch the existing upstream helper.

```bash
export OPBNB_MAINNET=https://opbnb-mainnet-rpc.bnbchain.org
read -rp 'Mainnet owner address: ' OPBNB_MAINNET_DEPLOYER_ADDRESS
read -rsp 'Mainnet deployer private key: ' OPBNB_MAINNET_DEPLOYER_PRIVKEY
export OPBNB_MAINNET_DEPLOYER_ADDRESS OPBNB_MAINNET_DEPLOYER_PRIVKEY
export DELEGATE_FEE=1000000000000000

cd contracts
forge create --broadcast --chain 204 \
    --rpc-url "$OPBNB_MAINNET" \
    --private-key "$OPBNB_MAINNET_DEPLOYER_PRIVKEY" \
    src/L2StandardBridgeBot.sol:L2StandardBridgeBot \
    --constructor-args "$OPBNB_MAINNET_DEPLOYER_ADDRESS" "$DELEGATE_FEE"
```

Record the returned helper address and deployment block in the bot config before
starting the bot with a fresh database for that deployment. Ensure your fee covers
the bot's expected L1 transaction costs.

To initiate a native BNB withdrawal later:

```bash
read -rp 'Deployed mainnet helper address: ' contract_addr
export amount=2000000000000001
cast send --chain 204 --rpc-url "$OPBNB_MAINNET" \
    --private-key "$OPBNB_MAINNET_DEPLOYER_PRIVKEY" \
    --value "$((DELEGATE_FEE + amount))" \
    "$contract_addr" 'withdrawTo(address,address,uint256,uint32,bytes)' \
    0xDeadDeAddeAddEAddeadDEaDDEAdDeaDDeAD0000 \
    "$OPBNB_MAINNET_DEPLOYER_ADDRESS" "$amount" 150469 0x
```

## License

MIT
