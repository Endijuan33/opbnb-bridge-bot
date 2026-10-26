package core

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	// defaultDialTimeout is default duration the processor will wait on
	// startup to make a connection to the backend
	defaultDialTimeout = 5 * time.Second

	// defaultDialAttempts is the default attempts a connection will be made
	// before failing
	defaultDialAttempts = 5

	// defaultRequestTimeout is the default duration the processor will
	// wait for a request to be fulfilled
	defaultRequestTimeout = 10 * time.Second
)

type ClientExt struct {
	ethclient.Client
}

var _ bind.ContractCaller = (*ClientExt)(nil)

func Dial(rawUrl string) (*ClientExt, error) {
	client, err := ethclient.DialContext(context.Background(), rawUrl)
	if err != nil {
		return nil, err
	}

	return &ClientExt{*client}, nil
}

// ValidateNetworks checks both RPCs before the bot indexes data or sends transactions.
func ValidateNetworks(ctx context.Context, cfg Config, l1Client, l2Client *ClientExt) error {
	for _, network := range []struct {
		name    string
		client  *ClientExt
		chainID uint64
	}{
		{"L1", l1Client, cfg.L1ChainID},
		{"L2", l2Client, cfg.L2ChainID},
	} {
		requestCtx, cancel := context.WithTimeout(ctx, defaultRequestTimeout)
		chainID, err := network.client.ChainID(requestCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("read %s chain ID: %w", network.name, err)
		}
		if chainID.Cmp(new(big.Int).SetUint64(network.chainID)) != 0 {
			return fmt.Errorf("%s chain ID mismatch: got %s, want %d", network.name, chainID, network.chainID)
		}
	}
	return nil
}

func (c *ClientExt) GetProof(address common.Address, storageKeys []string, blockNumber *big.Int) (*eth.AccountResult, error) {
	ctxwt, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()

	var result eth.AccountResult
	err := c.Client.Client().CallContext(ctxwt, &result, "eth_getProof", address, storageKeys, toBlockNumArg(blockNumber))
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// Impl bind.ContractCaller

func (c *ClientExt) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	var result string
	err := c.Client.Client().CallContext(ctx, &result, "eth_getCode", contract, toBlockNumArg(blockNumber))
	if err != nil {
		return nil, err
	}

	return hexutil.Decode(result)
}

func (c *ClientExt) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	var result string
	err := c.Client.Client().CallContext(ctx, &result, "eth_call", toCallArg(call), toBlockNumArg(blockNumber))
	if err != nil {
		return nil, err
	}

	return hexutil.Decode(result)
}

// New methods by block tag

func (c *ClientExt) GetHeaderByTag(ctx context.Context, blockTag string) (*types.Header, error) {
	var result types.Header
	err := c.Client.Client().CallContext(ctx, &result, "eth_getBlockByNumber", blockTag, false)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// Needed private utils from geth

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	if number.Sign() >= 0 {
		return hexutil.EncodeBig(number)
	}
	// It's negative.
	return rpc.BlockNumber(number.Int64()).String()
}

func toCallArg(msg ethereum.CallMsg) interface{} {
	arg := map[string]interface{}{
		"from": msg.From,
		"to":   msg.To,
	}
	if len(msg.Data) > 0 {
		arg["input"] = hexutil.Bytes(msg.Data)
	}
	if msg.Value != nil {
		arg["value"] = (*hexutil.Big)(msg.Value)
	}
	if msg.Gas != 0 {
		arg["gas"] = hexutil.Uint64(msg.Gas)
	}
	if msg.GasPrice != nil {
		arg["gasPrice"] = (*hexutil.Big)(msg.GasPrice)
	}
	return arg
}
