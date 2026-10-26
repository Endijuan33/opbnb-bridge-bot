package core

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/BurntSushi/toml"
	"github.com/ethereum-optimism/optimism/indexer/config"
	"github.com/ethereum/go-ethereum/log"
)

const (
	defaultLogFilterBlockRange = 100
	MainnetChallengeTimeWindow = 7 * 24 * 60 * 60
	L2StandardBridgeAddress    = "0x4200000000000000000000000000000000000010"
)

type Config struct {
	L1ChainID           uint64 `toml:"l1-chain-id"`
	L2ChainID           uint64 `toml:"l2-chain-id"`
	L2StandardBridge    string `toml:"l2-standard-bridge"`
	L2StartingNumber    int64  `toml:"l2-starting-number"`
	ProposeTimeWindow   int64  `toml:"propose-time-window"`
	ChallengeTimeWindow int64  `toml:"challenge-time-window"`

	RPCs                config.RPCsConfig         `toml:"rpcs"`
	DB                  config.DBConfig           `toml:"db"`
	L1Contracts         config.L1Contracts        `toml:"l1-contracts"`
	L2StandardBridgeBot L2StandardBridgeBotConfig `toml:"l2-standard-bridge-bot"`
	TxSigner            TxSignerConfig            `toml:"tx-signer"`
}

type TxSignerConfig struct {
	Privkey  string `toml:"-"`
	GasPrice int64  `toml:"gas-price"`
}

type L2StandardBridgeBotConfig struct {
	// ContractAddress is the address of the L2StandardBridgeBot contract.
	ContractAddress string `toml:"contract-address"`

	// LogFilterBlockRange is the number of blocks to filter for events
	LogFilterBlockRange int64 `toml:"log-filter-block-range"`

	// WhitelistL2TokenList is the list of L2 tokens to whitelist for the L2StandardBridgeBot contract.
	//
	// L2StandardBridgeBot contract doesn't limit the L2 tokens to be bridged, but this off-chain bot
	// process only whitelists the tokens in this list and ignore tokens not in this list.
	//
	// **IMPORTANT: If this list is unset, all L2 tokens are whitelisted.**
	WhitelistL2TokenList *[]string `toml:"whitelist-l2-token-list"`

	// UpperMinGasLimit is the upper limit of the minimum gas limit of the L2StandardBridgeBot contract.
	//
	// **IMPORTANT: If this value is unset, the L2StandardBridgeBot contract doesn't limit the minimum gas limit.**
	UpperMinGasLimit *uint32 `toml:"upper-min-gas-limit"`

	// UpperExtraDataSize is the upper limit of the extra data size of the L2StandardBridgeBot contract.
	//
	// **IMPORTANT: If this value is unset, the L2StandardBridgeBot contract doesn't limit the extra data size.**
	UpperExtraDataSize *uint32 `toml:"upper-extra-data-size"`
}

// LoadConfig loads the `bot.toml` config file from a given path
func LoadConfig(log log.Logger, path string) (Config, error) {
	log.Info("loading config", "path", path)

	var conf Config
	data, err := os.ReadFile(path)
	if err != nil {
		return conf, err
	}

	data = []byte(os.ExpandEnv(string(data)))
	if _, err := toml.Decode(string(data), &conf); err != nil {
		log.Info("failed to decode config file", "err", err)
		return conf, err
	}
	conf.TxSigner.Privkey = os.Getenv("OPBNB_BRIDGE_BOT_PRIVKEY")
	if conf.TxSigner.Privkey == "" {
		return conf, errors.New("OPBNB_BRIDGE_BOT_PRIVKEY must be set")
	}
	if conf.L1ChainID != 56 || conf.L2ChainID != 204 {
		return conf, errors.New("mainnet requires l1-chain-id = 56 and l2-chain-id = 204")
	}
	if !common.IsHexAddress(conf.L2StandardBridge) || common.HexToAddress(conf.L2StandardBridge) != common.HexToAddress(L2StandardBridgeAddress) {
		return conf, fmt.Errorf("l2-standard-bridge must be %s", L2StandardBridgeAddress)
	}
	if !common.IsHexAddress(conf.L2StandardBridgeBot.ContractAddress) || common.HexToAddress(conf.L2StandardBridgeBot.ContractAddress) == (common.Address{}) {
		return conf, errors.New("l2-standard-bridge-bot.contract-address must be a nonzero address")
	}
	if conf.RPCs.L1RPC == "" || conf.RPCs.L2RPC == "" {
		return conf, errors.New("l1-rpc and l2-rpc must be set")
	}
	if conf.L2StartingNumber < 0 {
		return conf, errors.New("l2-starting-number must be nonnegative")
	}

	if conf.L2StandardBridgeBot.LogFilterBlockRange == 0 {
		log.Info("setting default log filter block range", "log-filter-block-range", defaultLogFilterBlockRange)
		conf.L2StandardBridgeBot.LogFilterBlockRange = defaultLogFilterBlockRange
	}
	if conf.L2StandardBridgeBot.LogFilterBlockRange < 0 {
		return conf, errors.New("log-filter-block-range must be positive")
	}
	if conf.ProposeTimeWindow <= 0 {
		return conf, errors.New("propose-time-window must be set")
	}
	if conf.ChallengeTimeWindow != MainnetChallengeTimeWindow {
		return conf, errors.New("mainnet challenge-time-window must be 604800 seconds (7 days)")
	}

	if _, _, err = conf.SignerKeyPair(); err != nil {
		return conf, err
	}
	if conf.TxSigner.GasPrice <= 0 {
		return conf, errors.New("gas-price must be set")
	}

	log.Info("loaded config")
	return conf, nil
}

func (c *Config) SignerKeyPair() (*ecdsa.PrivateKey, *common.Address, error) {
	privkey, err := crypto.HexToECDSA(strings.TrimPrefix(c.TxSigner.Privkey, "0x"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse privkey: %w", err)
	}

	pubKey := privkey.Public()
	pubKeyECDSA, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, nil, errors.New("failed to cast public key to ECDSA")
	}

	pubKeyBytes := crypto.FromECDSAPub(pubKeyECDSA)
	pubKeyHash := crypto.Keccak256(pubKeyBytes[1:])[12:]
	address := common.HexToAddress(hexutil.Encode(pubKeyHash))
	return privkey, &address, nil
}
