package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	rpcURL        = flag.String("rpc", "", "RPC URL of the Ethereum network")
	privateKey    = flag.String("key", "", "Private key for deployment (without 0x prefix)")
	tokenName     = flag.String("name", "", "Name of the token")
	tokenSymbol   = flag.String("symbol", "", "Symbol of the token")
	tokenDecimals = flag.Uint("decimals", 18, "Number of decimals for the token")
	totalSupply   = flag.String("supply", "", "Total supply of tokens (in whole units)")
	gasLimit      = flag.Uint64("gas", 3000000, "Gas limit for deployment")
	gasPriceGwei  = flag.Float64("gasprice", 0, "Gas price in Gwei (optional)")
)

func main() {
	flag.Parse()

	if *rpcURL == "" || (*privateKey == "" && !promptForPrivateKey()) || *tokenName == "" || *tokenSymbol == "" || *totalSupply == "" {
		log.Fatal("All flags are required: -rpc, -key, -name, -symbol, -supply")
	}

	client, err := ethclient.Dial(*rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum network: %v", err)
	}
	defer client.Close()

	auth, err := createTransactor(*privateKey, client)
	if err != nil {
		log.Fatalf("Failed to create transactor: %v", err)
	}

	supply, err := parseSupply(*totalSupply, uint8(*tokenDecimals))
	if err != nil {
		log.Fatalf("Failed to parse supply: %v", err)
	}

	address, tx, instance, err := DeployERC20Token(
		auth,
		client,
		*tokenName,
		*tokenSymbol,
		uint8(*tokenDecimals),
		supply,
	)
	if err != nil {
		log.Fatalf("Failed to deploy contract: %v", err)
	}

	fmt.Printf("Token deployment initiated!\n")
	fmt.Printf("Contract address: %s\n", address.Hex())
	fmt.Printf("Transaction hash: %s\n", tx.Hash().Hex())
	fmt.Printf("Waiting for transaction to be mined...\n")

	receipt, err := bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		log.Fatalf("Failed to wait for mining: %v", err)
	}

	if receipt.Status == 1 {
		fmt.Printf("\nDeployment successful!\n")
		fmt.Printf("Gas used: %d\n", receipt.GasUsed)

		name, err := instance.Name(&bind.CallOpts{})
		if err == nil {
			fmt.Printf("Token name: %s\n", name)
		}
		symbol, err := instance.Symbol(&bind.CallOpts{})
		if err == nil {
			fmt.Printf("Token symbol: %s\n", symbol)
		}
		decimals, err := instance.Decimals(&bind.CallOpts{})
		if err == nil {
			fmt.Printf("Token decimals: %d\n", decimals)
		}
	} else {
		fmt.Printf("\nDeployment failed! Check the transaction on a block explorer.\n")
	}
}

func createTransactor(privateKeyHex string, client *ethclient.Client) (*bind.TransactOpts, error) {
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %v", err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("error casting public key to ECDSA")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %v", err)
	}

	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)

	if *gasPriceGwei > 0 {
		gasPriceWei := new(big.Int).Mul(big.NewInt(int64(*gasPriceGwei*1e9)), big.NewInt(1))
		auth.GasPrice = gasPriceWei
	} else {
		gasPrice, err := client.SuggestGasPrice(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to suggest gas price: %v", err)
		}
		auth.GasPrice = gasPrice
	}

	auth.GasLimit = *gasLimit

	return auth, nil
}

func parseSupply(supply string, decimals uint8) (*big.Int, error) {
	value := new(big.Int)
	_, ok := value.SetString(supply, 10)
	if !ok {
		return nil, fmt.Errorf("invalid supply value: %s", supply)
	}

	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	return value.Mul(value, multiplier), nil
}

func promptForPrivateKey() bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter your private key (without 0x prefix): ")
	key, err := reader.ReadString('\n')
	if err != nil {
		log.Fatalf("Failed to read private key: %v", err)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	*privateKey = key
	return true
}
