package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/BlockRazorinc/sui_go_example/blockrzsdk"

	"github.com/block-vision/sui-go-sdk/models"
	"github.com/block-vision/sui-go-sdk/signer"
	"github.com/block-vision/sui-go-sdk/sui"
	"github.com/block-vision/sui-go-sdk/transaction"
)

const (
	// Official Sui fullnode RPC (mainnet)
	OfficialRpcUrl = "https://fullnode.mainnet.sui.io"
	// Gas object id
	GasObjectId = "<YOUR_GAS_OBJECT_ID>"
	// Receiver address
	ReceiverAddr = "<YOUR_RECEIVER_ADDRESS>"
	// Transfer amount in MIST
	SendAmountMist = uint64(100000)
	// Default gas price
	GasPrice = uint64(5000)
	// BlockRz RPC endpoint
	BlockRzRpcUrl = "<YOUR_BLOCKRZ_RPC_URL>"
	// Auth token header for your RPC
	AuthToken = "<YOUR_AUTH_TOKEN>"
	// Timeouts
	BuildTimeout = 5 * time.Second
	HttpTimeout  = 15 * time.Second
)

// fetchLatestObjectRef fetches the latest (version, digest) for an objectId
func fetchLatestObjectRef(ctx context.Context, cli *sui.Client, objectId string) (*transaction.SuiObjectRef, error) {
	obj, err := cli.SuiGetObject(ctx, models.SuiGetObjectRequest{ObjectId: objectId})
	if err != nil {
		return nil, fmt.Errorf("SuiGetObject failed: %w", err)
	}
	if obj.Data == nil {
		return nil, fmt.Errorf("object not found or deleted: %s", objectId)
	}
	if obj.Data.Version == "" || obj.Data.Digest == "" {
		return nil, fmt.Errorf("invalid object data: id=%s version=%s digest=%s", objectId, obj.Data.Version, obj.Data.Digest)
	}

	ref, err := transaction.NewSuiObjectRef(
		models.SuiAddress(objectId),
		obj.Data.Version,
		models.ObjectDigest(obj.Data.Digest),
	)
	if err != nil {
		return nil, fmt.Errorf("NewSuiObjectRef failed: %w", err)
	}
	return ref, nil
}

// makeHttpRequest builds the JSON-RPC request for `sui_executeTransactionBlock`.
func makeHttpRequest(tx *transaction.Transaction) (*http.Request, []byte, error) {
	reqModel, err := tx.ToSuiExecuteTransactionBlockRequest(
		context.Background(),
		models.SuiTransactionBlockOptions{ShowEffects: true},
		"WaitForLocalExecution",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("ToSuiExecuteTransactionBlockRequest failed: %w", err)
	}

	bodyMap := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sui_executeTransactionBlock",
		"params": []any{
			reqModel.TxBytes,
			reqModel.Signature,
			reqModel.Options,
			reqModel.RequestType,
		},
	}

	jsonBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, nil, fmt.Errorf("json.Marshal failed: %w", err)
	}
	req, err := http.NewRequest("POST", BlockRzRpcUrl, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("NewRequest failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("auth_token", AuthToken)
	rawTxBytes, err := base64.StdEncoding.DecodeString(reqModel.TxBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("base64 decode TxBytes failed: %w", err)
	}
	return req, rawTxBytes, nil
}

// buildBaseTx creates a basic transfer tx: split from gas coin then transfer to receiver.
func buildBaseTx(cli *sui.Client, s *signer.Signer, gasRef *transaction.SuiObjectRef) *transaction.Transaction {
	tx := transaction.NewTransaction()
	tx.SetSuiClient(cli).
		SetSigner(s).
		SetSender(models.SuiAddress(s.Address)).
		SetGasPayment([]transaction.SuiObjectRef{*gasRef}).
		SetGasOwner(models.SuiAddress(s.Address))

	split := tx.SplitCoins(tx.Gas(), []transaction.Argument{tx.Pure(SendAmountMist)})
	tx.TransferObjects([]transaction.Argument{split}, tx.Pure(ReceiverAddr))
	return tx
}

func main() {
	ctx := context.Background()
	// Build signer from secret key
	secretKey := "<YOUR_SECRET_KEY>"
	sender, err := signer.NewSignerWithSecretKey(secretKey)
	if err != nil {
		log.Fatalf("[INIT] NewSignerWithSecretKey failed: %v", err)
	}
	log.Printf("[INIT] sender=%s receiver=%s amountMist=%d", sender.Address, ReceiverAddr, SendAmountMist)

	// Create Sui client (official fullnode)
	cli := sui.NewSuiClient(OfficialRpcUrl).(*sui.Client)

	// Fetch latest gas object ref
	gasRef, err := fetchLatestObjectRef(ctx, cli, GasObjectId)
	if err != nil {
		log.Fatalf("[GAS] fetchLatestObjectRef failed: %v", err)
	}
	log.Printf("[GAS] gasObject=%s version=%d digest=%s", GasObjectId, gasRef.Version, gasRef.Digest)

	// Build base tx
	tx := buildBaseTx(cli, sender, gasRef)

	// ============================================
	//  Estimate fee & tip using dryRun
	// ============================================
	tx.SetGasPrice(GasPrice)
	buildCtx, cancel := context.WithTimeout(context.Background(), BuildTimeout)
	defer cancel()

	// Build TxBytes for estimation
	reqModel, err := tx.ToSuiExecuteTransactionBlockRequest(
		buildCtx,
		models.SuiTransactionBlockOptions{},
		"WaitForLocalExecution",
	)
	if err != nil {
		log.Fatalf("[PREP] build tx bytes failed: %v", err)
	}

	// Caculate Fee
	est, err := blockrzsdk.CalculateFeeFromTxB64(buildCtx, reqModel.TxBytes)
	if err != nil {
		log.Fatalf("[PREP] CalculateFeeFromTxB64 failed: %v", err)
	}
	tx.SetGasBudget(est.GasBudget)

	// Add tip into transaction
	if err := blockrzsdk.AddTip(tx, est.TipAmount); err != nil {
		log.Fatalf("[PREP] AddTip failed: %v", err)
	}
	log.Printf("[PREP] gasPrice=%d gasBudget=%d tip=%d txBytesLen=%d",
		GasPrice, est.GasBudget, est.TipAmount, len(reqModel.TxBytes))

	// ============================================
	//  Build HTTP request and Send to BlockRz
	// ============================================
	req, _, err := makeHttpRequest(tx)
	if err != nil {
		log.Fatalf("[HTTP] makeHttpRequest failed: %v", err)
	}

	// Send HTTP request and read response body
	client := &http.Client{Timeout: HttpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("[HTTP] request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("[HTTP] read response body failed: %v", err)
	}
	log.Printf("[HTTP][RESP] body=%s", body)
}
