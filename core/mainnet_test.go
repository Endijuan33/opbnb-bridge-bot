package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	botbindings "bnbchain/opbnb-bridge-bot/bindings"
	"github.com/ethereum-optimism/optimism/indexer/config"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
)

func mainnetTestEnv(t *testing.T) string {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	keyHex := common.Bytes2Hex(crypto.FromECDSA(key))
	t.Setenv("OPBNB_BRIDGE_BOT_PRIVKEY", keyHex)
	t.Setenv("OPBNB_BRIDGE_BOT_DB_HOST", "127.0.0.1")
	t.Setenv("OPBNB_BRIDGE_BOT_DB_PORT", "3306")
	t.Setenv("OPBNB_BRIDGE_BOT_DB_USER", "test")
	t.Setenv("OPBNB_BRIDGE_BOT_DB_PASSWORD", "test-password")
	return keyHex
}

func TestMainnetConfigAndSecretLogging(t *testing.T) {
	keyHex := mainnetTestEnv(t)
	var output bytes.Buffer
	logger := log.New()
	logger.SetHandler(log.StreamHandler(&output, log.LogfmtFormat()))
	cfg, err := LoadConfig(logger, "../bot.mainnet.toml")
	require.NoError(t, err)
	require.Equal(t, uint64(56), cfg.L1ChainID)
	require.Equal(t, uint64(204), cfg.L2ChainID)
	require.Equal(t, "https://bsc-dataseed.bnbchain.org", cfg.RPCs.L1RPC)
	require.Equal(t, "https://opbnb-mainnet-rpc.bnbchain.org", cfg.RPCs.L2RPC)
	require.EqualValues(t, 604800, cfg.ChallengeTimeWindow)
	require.Equal(t, "opbnb_bridge_bot_mainnet", cfg.DB.Name)
	require.Equal(t, keyHex, cfg.TxSigner.Privkey)
	require.NotContains(t, output.String(), keyHex)
	require.NotContains(t, output.String(), "test-password")

	t.Setenv("OPBNB_BRIDGE_BOT_PRIVKEY", "0x"+keyHex)
	cfg, err = LoadConfig(logger, "../bot.mainnet.toml")
	require.NoError(t, err)
	_, _, err = cfg.SignerKeyPair()
	require.NoError(t, err)
}

func TestMainnetConfigRejectsWrongNetworkAndTiming(t *testing.T) {
	mainnetTestEnv(t)
	original, err := os.ReadFile("../bot.mainnet.toml")
	require.NoError(t, err)
	for _, tc := range []struct{ name, old, replacement, want string }{
		{"L1 testnet", "l1-chain-id = 56", "l1-chain-id = 97", "l1-chain-id = 56"},
		{"L2 testnet", "l2-chain-id = 204", "l2-chain-id = 5611", "l2-chain-id = 204"},
		{"short challenge", "challenge-time-window = 604800", "challenge-time-window = 120", "604800 seconds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bot.toml")
			require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(original), tc.old, tc.replacement, 1)), 0600))
			_, err := LoadConfig(log.New(), path)
			require.ErrorContains(t, err, tc.want)
		})
	}
	t.Setenv("OPBNB_BRIDGE_BOT_PRIVKEY", "")
	_, err = LoadConfig(log.New(), "../bot.mainnet.toml")
	require.ErrorContains(t, err, "OPBNB_BRIDGE_BOT_PRIVKEY must be set")
}

func mockChainClient(t *testing.T, chainID uint64) *ClientExt {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string
			ID     json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if request.Method != "eth_chainId" {
			http.Error(w, "unexpected RPC method", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x%x"}`, request.ID, chainID)
	}))
	t.Cleanup(server.Close)
	client, err := Dial(server.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

func TestValidateNetworks(t *testing.T) {
	cfg := Config{L1ChainID: 56, L2ChainID: 204}
	require.NoError(t, ValidateNetworks(context.Background(), cfg, mockChainClient(t, 56), mockChainClient(t, 204)))
	require.ErrorContains(t, ValidateNetworks(context.Background(), cfg, mockChainClient(t, 97), mockChainClient(t, 204)), "L1 chain ID mismatch")
	require.ErrorContains(t, ValidateNetworks(context.Background(), cfg, mockChainClient(t, 56), mockChainClient(t, 5611)), "L2 chain ID mismatch")
}

func mainnetReceipt(t *testing.T) *types.Receipt {
	t.Helper()
	data, err := os.ReadFile("testdata/mainnet-withdrawal-receipt.json")
	require.NoError(t, err)
	var receipt types.Receipt
	require.NoError(t, json.Unmarshal(data, &receipt))
	return &receipt
}

func mainnetProcessor() *Processor {
	return NewProcessor(log.New(), nil, nil, Config{
		L2StandardBridge:    L2StandardBridgeAddress,
		L1Contracts:         config.L1Contracts{L1CrossDomainMessengerProxy: common.HexToAddress("0xd95D508f13f7029CCF0fb61984d5dfD11b879c4f")},
		L2StandardBridgeBot: L2StandardBridgeBotConfig{ContractAddress: "0x4000698e3De52120DE28181BaACda82B21568416"},
	})
}

// Captured from opBNB mainnet block 13113334, transaction
// 0x77cc9c7392442c289d30112032203e387e0fe04273a6eafe5863430aa5900c07.
// This verifies decoding and reconstruction offline without sending transactions.
func TestDeployedMainnetWithdrawToAndReconstruction(t *testing.T) {
	receipt := mainnetReceipt(t)
	processor := mainnetProcessor()
	helperLog := receipt.Logs[5]
	require.Equal(t, WithdrawToEventSig(), helperLog.Topics[0])
	require.Len(t, helperLog.Topics, 3)
	filterer, err := botbindings.NewL2StandardBridgeBotFilterer(helperLog.Address, nil)
	require.NoError(t, err)
	event, err := filterer.ParseWithdrawTo(*helperLog)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress("0xa3b5c58cef73ada157ceaecdeaa68d7ea2fc0592"), event.From)
	require.Equal(t, common.HexToAddress("0xDeadDeAddeAddEAddeadDEaDDEAdDeaDDeAD0000"), event.L2Token)
	indexer := NewIndexer(log.New(), nil, nil, processor.cfg)
	require.True(t, indexer.isL2StandardBridgeBotWithdrawToEvent(helperLog))
	require.True(t, indexer.isL2StandardBridgeWithdrawalInitiatedLog(receipt.Logs[0]))

	withdrawal, err := processor.toWithdrawal(&WithdrawalInitiatedLogV2{LogIndex: 0}, receipt)
	require.NoError(t, err)
	require.Equal(t, event.Amount, withdrawal.Value)
	message, err := processor.toMessagePassed(receipt.Logs[2])
	require.NoError(t, err)
	hash, err := processor.hashWithdrawal(withdrawal)
	require.NoError(t, err)
	require.Equal(t, common.Hash(message.WithdrawalHash), common.HexToHash(hash))
	require.Equal(t, message.Data, withdrawal.Data)
	require.NoError(t, processor.CheckByFilterOptions(receipt.Logs[0], withdrawal))

	processor.cfg.L1Contracts.L1CrossDomainMessengerProxy = common.Address{}
	_, err = processor.toWithdrawal(&WithdrawalInitiatedLogV2{LogIndex: 0}, receipt)
	require.ErrorContains(t, err, "does not match MessagePassed")
}

func TestReconstructionRejectsInvalidMessageAndHash(t *testing.T) {
	receipt := mainnetReceipt(t)
	processor := mainnetProcessor()
	sent, err := processor.toL2CrossDomainMessengerSentMessageExtension1(receipt.Logs[3], receipt.Logs[4])
	require.NoError(t, err)
	message, err := processor.toMessagePassed(receipt.Logs[2])
	require.NoError(t, err)
	message.WithdrawalHash[0] ^= 1
	_, err = processor.toLowLevelMessage(sent, message)
	require.ErrorContains(t, err, "withdrawal hash does not match")
	receipt.Logs[2].Topics = nil
	_, err = processor.toWithdrawal(&WithdrawalInitiatedLogV2{LogIndex: 0}, receipt)
	require.ErrorContains(t, err, "not MessagePassed event")
}
