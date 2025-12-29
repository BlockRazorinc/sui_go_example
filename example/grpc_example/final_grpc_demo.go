package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"github.com/BlockRazorinc/sui_go_example/blockrzsdk"
	pb "github.com/BlockRazorinc/sui_go_example/sui/rpc/v2"

	"github.com/block-vision/sui-go-sdk/models"
	"github.com/block-vision/sui-go-sdk/signer"
	"github.com/block-vision/sui-go-sdk/sui"
	"github.com/block-vision/sui-go-sdk/transaction"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	OfficialRpcUrl = "https://fullnode.mainnet.sui.io"
	// Gas object
	GasObjectId = "<YOUR_GAS_OBJECT_ID>"
	// Receiver address
	ReceiverAddr = "<YOUR_RECEIVER_ADDRESS>"
	// Transfer amount in MIST
	SendAmountMist = uint64(100000)
	// Default gas price (can be fetched from RPC if needed)
	GasPrice = uint64(5000)
	// BlockRz GRPC endpoint
	BlockRzGrpcUrl = "<YOUR_BLOCKRZ_GRPC_URL>"
	// Auth token header for your RPC
	AuthToken = "<YOUR_AUTH_TOKEN>"
	// Timeouts
	BuildTimeout = 5 * time.Second
	GrpcTimeout  = 15 * time.Second
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

// makeGrpcRequest converts transaction into gRPC ExecuteTransactionRequest
func makeGrpcRequest(ctx context.Context, tx *transaction.Transaction) (*pb.ExecuteTransactionRequest, []byte, error) {
	// Align with HTTP demo: use WaitForLocalExecution
	reqModel, err := tx.ToSuiExecuteTransactionBlockRequest(
		ctx,
		models.SuiTransactionBlockOptions{},
		"WaitForLocalExecution",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("ToSuiExecuteTransactionBlockRequest failed: %w", err)
	}

	txBytes, err := base64.StdEncoding.DecodeString(reqModel.TxBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("base64 decode TxBytes failed: %w", err)
	}

	if len(reqModel.Signature) == 0 {
		return nil, nil, fmt.Errorf("empty signature in reqModel")
	}

	sigBytes, err := base64.StdEncoding.DecodeString(reqModel.Signature[0])
	if err != nil {
		return nil, nil, fmt.Errorf("base64 decode signature failed: %w", err)
	}

	return &pb.ExecuteTransactionRequest{
		Transaction: &pb.Transaction{Bcs: &pb.Bcs{Value: txBytes}},
		Signatures:  []*pb.UserSignature{{Bcs: &pb.Bcs{Value: sigBytes}}},
	}, txBytes, nil
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

	reqModel, err := tx.ToSuiExecuteTransactionBlockRequest(
		buildCtx,
		models.SuiTransactionBlockOptions{}, // dryRun no need showEffects
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
	//  Build gRPC request and Send to BlockRz
	// ============================================
	conn, err := grpc.Dial(BlockRzGrpcUrl, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("[GRPC] dial failed: %v", err)
	}
	defer conn.Close()

	grpcClient := pb.NewTransactionExecutionServiceClient(conn)

	grpcCtx, cancel := context.WithTimeout(context.Background(), GrpcTimeout)
	defer cancel()

	grpcReq, txBytes, err := makeGrpcRequest(grpcCtx, tx)
	if err != nil {
		log.Fatalf("[GRPC] makeGrpcRequest failed: %v", err)
	}
	log.Printf("[GRPC][REQ] txBytesLen=%d sigCount=%d", len(txBytes), len(grpcReq.Signatures))

	// Attach auth token
	md := metadata.Pairs("auth_token", AuthToken)
	grpcCtx = metadata.NewOutgoingContext(grpcCtx, md)

	start := time.Now()
	resp, err := grpcClient.ExecuteTransaction(grpcCtx, grpcReq)
	if err != nil {
		log.Fatalf("[GRPC] ExecuteTransaction failed: %v", err)
	}

	log.Printf("[GRPC][RESP] cost=%s resp=%+v", time.Since(start), resp)
}
