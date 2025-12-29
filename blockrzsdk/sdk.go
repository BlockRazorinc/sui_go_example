package blockrzsdk

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/block-vision/sui-go-sdk/models"
	"github.com/block-vision/sui-go-sdk/sui"
	"github.com/block-vision/sui-go-sdk/transaction"
)

const (
	// BlockRazor tipmanager package
	BlockRzPackageId = "0xc07e7aac927814d8fd4f532d1c4a6216a5ecc20df3dc5d2967b3994f87ff6e87"

	// Sui framework
	SuiPackageId = "0x2"
	SuiCoinType  = "0x2::sui::SUI"

	// Default official mainnet fullnode RPC
	DefaultMainnetRPC = "https://fullnode.mainnet.sui.io:443"

	// Default execute request type
	DefaultRequestType = "WaitForLocalExecution"
)

type SharedTipObject struct {
	ObjectId string
	Version  uint64
}

var BlockRzTipsObjects = []SharedTipObject{
	{ObjectId: "0x188c9ea21b58b07fab5afdd0b30ffc33d5af74781454d1b008ee4ab620652fad", Version: 730767796},
	{ObjectId: "0xe5153b62740b525424c9b71b590b4fef1e018d14280fb7ba76f4ba0573cef875", Version: 730767797},
	{ObjectId: "0xff8ffbf8dd417d5ee5c3eafc01cda144bb07da8b4b72d4ec5fa85c1224bf6a86", Version: 730767799},
	{ObjectId: "0x982f533710e83d674eb8f2a76fddfa0045ede3b8debdcbd06833f81deba43954", Version: 730767801},
	{ObjectId: "0x8340398a9423ff01807e9fe2833ec790f8ccda8cea44853d2a1143dca4d47d48", Version: 730767803},
	{ObjectId: "0xb9b8c50fa031b5bc1a2240ffef79d0e4d13cf7f50f2e3a0ad5dfca48db835be9", Version: 730767805},
	{ObjectId: "0x533ebf862a2174b9df51ad0b6bef623639385268848f4c8fabbe550e0105e76a", Version: 730767807},
	{ObjectId: "0x9cf05f5a8c639ab4e4b6bc5d79becce7412ee39d456f2151686dc48cec2c50ca", Version: 730767809},
	{ObjectId: "0xd9a87b4ae9515ce28d04ae86882724027f1cf261c6cdecd974e0118dcdf33bbc", Version: 730767811},
	{ObjectId: "0x4ee9abaa304c741501b6fd99821dd7e266583585c5f16579f6011b763c91d9e5", Version: 730767813},
	{ObjectId: "0x3243eb0b3e2ec063e0216af50ca0fadcec6335f30349d859f6cfd652fdbaa3f3", Version: 730767815},
	{ObjectId: "0x3290a3b00596d891c48570ecf87eec34a5974ab766915a3148e1017c6bf604a0", Version: 730767817},
	{ObjectId: "0x0682d120eb5674cd092ddbdd31881000da4b469cb6ac2eda3c55f21b72db6810", Version: 730767819},
	{ObjectId: "0x5661c7c2405820a14e1a1d159847d302dd893b4b582efc0c166dfef5ee7b25fa", Version: 730767821},
	{ObjectId: "0x0ff2ba21bdfb2e5b360177de4df89d34f2f7594ce4185e9faa6f7d35e75669c2", Version: 730767823},
	{ObjectId: "0xe341141150e28b479bd0514fec9a6e8326f3d75b07d60f00829d63010d334f12", Version: 730767825},
	{ObjectId: "0x423b925119bbeece545e79dc1cea95bed121c99f1ddab32602654c4dea8c89f0", Version: 730767827},
	{ObjectId: "0xe396ab823b1d85dc0f8428a915715becf10b49214e8d8068516197d858ff703a", Version: 730767829},
	{ObjectId: "0x3de1876d8302a7dd1112c9847c617228a61a3b095ee9e1a9c66965c56edc3293", Version: 730767831},
	{ObjectId: "0xf8492eda49bc53bd5fa5f3e1b18eafea303ff8c7776f2059b7c1c540ee459d95", Version: 730767833},
	{ObjectId: "0x68bacf67761dd2d0bf811ce3970c649415ff111d0975cf90dc1ecf17aaebf18e", Version: 730767835},
	{ObjectId: "0x853d4302d4aca976ab4c783d03836cf4b23220a67ee650bc72addcec93311ea3", Version: 730767837},
	{ObjectId: "0x13b4f8a529014c76c97bd6fd96cd685f151ca9f858a2b0b1a43665a2311b78a0", Version: 730767839},
	{ObjectId: "0x5511d6f18df30f78bbc18f0f67230bdb3e9b807345d357f5427babfaf8bb2973", Version: 730767841},
	{ObjectId: "0x27d27f1546069d5cdcbd1343b281023ab3c5aa0bb3c266124361f7e416ced66c", Version: 730767843},
	{ObjectId: "0x83f85ea5e76b4d1cd4e9f5a6583ee56afd7c1bc9c9612913d582ce8b833d9de2", Version: 730767845},
}

var (
	defaultClientOnce sync.Once
	defaultClient     sui.ISuiAPI

	rngOnce sync.Once
	rng     *rand.Rand
)

func getDefaultClient() sui.ISuiAPI {
	defaultClientOnce.Do(func() {
		defaultClient = sui.NewSuiClient(DefaultMainnetRPC)
	})
	return defaultClient
}

func getRng() *rand.Rand {
	rngOnce.Do(func() {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	})
	return rng
}

type CalculatedFee struct {
	GasBudget uint64 `json:"gasBudget"`
	TipAmount uint64 `json:"tipAmount"`
}

type ExecuteResult = models.SuiTransactionBlockResponse

func mustBigIntDecimal(s, label string) *big.Int {
	z, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic(fmt.Sprintf("invalid %s: %q", label, s))
	}
	return z
}

func ceilMulDiv(x *big.Int, mul int64, div int64) *big.Int {
	m := big.NewInt(mul)
	d := big.NewInt(div)

	num := new(big.Int).Mul(x, m)
	num.Add(num, new(big.Int).Sub(d, big.NewInt(1)))
	return num.Div(num, d)
}

func CalBudgetAndTip(g models.GasCostSummary) (gasBudget uint64, tipAmount uint64) {
	comp := mustBigIntDecimal(g.ComputationCost, "computationCost")
	stor := mustBigIntDecimal(g.StorageCost, "storageCost")

	gross := new(big.Int).Add(comp, stor)

	gb := ceilMulDiv(gross, 120, 100) // *1.2 ceil
	tip := ceilMulDiv(gb, 5, 100)     // 5% ceil

	if gb.BitLen() > 64 {
		panic(fmt.Sprintf("gasBudget exceeds uint64: %s", gb.String()))
	}
	if tip.BitLen() > 64 {
		panic(fmt.Sprintf("tipAmount exceeds uint64: %s", tip.String()))
	}
	return gb.Uint64(), tip.Uint64()
}

func CalculateFeeFromTxBytes(ctx context.Context, txBytes []byte) (*CalculatedFee, error) {
	txB64 := base64.StdEncoding.EncodeToString(txBytes)
	return CalculateFeeFromTxB64(ctx, txB64)
}

func CalculateFeeFromTxB64(ctx context.Context, txB64 string) (*CalculatedFee, error) {
	c := getDefaultClient()

	resp, err := c.SuiDryRunTransactionBlock(ctx, models.SuiDryRunTransactionBlockRequest{
		TxBytes: txB64,
	})
	if err != nil {
		return nil, err
	}

	gb, tip := CalBudgetAndTip(resp.Effects.GasUsed)
	return &CalculatedFee{GasBudget: gb, TipAmount: tip}, nil
}

func GetBlockRzTipsObject() SharedTipObject {
	r := getRng()
	return BlockRzTipsObjects[r.Intn(len(BlockRzTipsObjects))]
}

func AddTip(tx *transaction.Transaction, tipAmount uint64) error {
	// split gas coin
	tipCoin := tx.SplitCoins(tx.Gas(), []transaction.Argument{
		tx.Pure(tipAmount),
	})

	suiPkg, err := transaction.ConvertSuiAddressStringToBytes(models.SuiAddress(SuiPackageId))
	if err != nil {
		return err
	}
	suiType := transaction.TypeTag{
		Struct: &transaction.StructTag{
			Address: *suiPkg,
			Module:  "sui",
			Name:    "SUI",
		},
	}
	balance := tx.MoveCall(
		SuiPackageId,
		"coin",
		"into_balance",
		[]transaction.TypeTag{suiType},
		[]transaction.Argument{tipCoin},
	)

	obj := GetBlockRzTipsObject()
	sharedArg, err := addSharedObjectInput(tx, obj.ObjectId, obj.Version)
	if err != nil {
		return err
	}
	// call tipmanager::add_tip
	tx.MoveCall(
		BlockRzPackageId,
		"tipmanager",
		"add_tip",
		nil,
		[]transaction.Argument{
			sharedArg,
			tx.Pure(tipAmount),
			balance,
		},
	)

	return nil
}
func addSharedObjectInput(tx *transaction.Transaction, objId string, version uint64) (transaction.Argument, error) {
	objAddrBytes, err := transaction.ConvertSuiAddressStringToBytes(models.SuiAddress(objId))
	if err != nil {
		return transaction.Argument{}, err
	}
	callArg := transaction.CallArg{
		Object: &transaction.ObjectArg{
			SharedObject: &transaction.SharedObjectRef{ObjectId: *objAddrBytes, InitialSharedVersion: version, Mutable: true},
		},
	}
	return tx.Data.V1.AddInput(callArg), nil
}
