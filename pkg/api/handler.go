package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tonkeeper/claim-api-go/pkg/prover"
	boc "github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/contract/jetton"
	"github.com/tonkeeper/tongo/liteapi"
	"github.com/tonkeeper/tongo/tlb"
	"github.com/tonkeeper/tongo/ton"
	"go.uber.org/zap"

	"github.com/tonkeeper/claim-api-go/pkg/api/oas"
)

// Handler handles operations described by OpenAPI v3 specification of this service.
// It implements oas.Handler interface and every API operation is implemented as a method on Handler.
type Handler struct {
	logger *zap.Logger

	prover       *prover.Prover
	jettonMaster ton.AccountID
}

type Config struct {
	AirdropFilename string
	JettonMaster    ton.AccountID
}

var _ oas.Handler = (*Handler)(nil)

func NewHandler(logger *zap.Logger, config Config) (*Handler, error) {
	cli, err := liteapi.NewClient(liteapi.FromEnvs())
	if err != nil {
		return nil, err
	}
	_ = cli
	jettonMaster := jetton.New(config.JettonMaster, cli)
	_ = jettonMaster

	proverConfig := prover.Config{
		Filename: config.AirdropFilename,
	}
	p, err := prover.NewProver(logger, proverConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create prover: %w", err)
	}
	return &Handler{
		prover:       p,
		logger:       logger,
		jettonMaster: config.JettonMaster,
	}, nil
}

func (h *Handler) NewError(ctx context.Context, err error) *oas.ErrorStatusCode {
	switch x := err.(type) {
	case *oas.ErrorStatusCode:
		return x
	default:
		return InternalError(x)
	}
}

func (h *Handler) Run(ctx context.Context) {
	go h.prover.Run(ctx)
}

func (h *Handler) GetWalletInfo(ctx context.Context, params oas.GetWalletInfoParams) (*oas.WalletInfo, error) {
	accountID, err := ton.ParseAccountID(params.Address)
	if err != nil {
		return nil, BadRequest("failed to parse account id")
	}
	msgAddr := accountID.ToMsgAddress()
	c := boc.NewCell()
	if err := tlb.Marshal(c, msgAddr); err != nil {
		return nil, InternalError(err)
	}
	c.ResetCounters()
	responseCh := make(chan prover.ProofResponse, 1)
	h.prover.Queue() <- prover.ProofRequest{
		AccountID:  c,
		ResponseCh: responseCh,
	}
	select {
	case <-ctx.Done():
		return nil, BadRequest("timeout")
	case resp := <-responseCh:
		if strings.Contains(resp.Err.Error(), "key is not found") {
			return nil, NotFound("account not not found")
		}
		if resp.Err != nil {
			return nil, InternalError(resp.Err)
		}
		customPayload, err := createCustomPayload(resp.Proof)
		if err != nil {
			return nil, InternalError(err)
		}
		stateInit, err := createStateInit(accountID, h.jettonMaster, h.prover.MerkleRoot())
		if err != nil {
			return nil, InternalError(err)
		}

		compressedInfo := oas.WalletInfoCompressedInfo{
			Amount:    strconv.FormatUint(uint64(resp.AirdropPayload.Amount), 10),
			StartFrom: strconv.FormatUint(uint64(resp.AirdropPayload.StartFrom), 10),
			ExpiredAt: strconv.FormatUint(uint64(resp.AirdropPayload.ExpireAt), 10),
		}
		return &oas.WalletInfo{
			Owner:          accountID.ToRaw(),
			CustomPayload:  customPayload,
			StateInit:      oas.NewOptString(stateInit),
			CompressedInfo: oas.NewOptWalletInfoCompressedInfo(compressedInfo),
		}, nil
	}
}

func createCustomPayload(proof []byte) (string, error) {
	proofCells, err := boc.DeserializeBoc(proof)
	if err != nil {
		return "", err
	}
	if len(proofCells) != 1 {
		return "", fmt.Errorf("proof is broken")
	}
	customPayload := boc.NewCell()
	if err := customPayload.WriteUint(0x0df602d6, 32); err != nil {
		return "", err
	}
	if err := customPayload.AddRef(proofCells[0]); err != nil {
		return "", err
	}
	return customPayload.ToBocBase64()
}

type JettonData struct {
	Status              tlb.Uint4
	Balance             tlb.Grams
	OwnerAddress        tlb.MsgAddress
	JettonMasterAddress tlb.MsgAddress
	MerkleRoot          tlb.Bits256
}

func createStateInit(owner, minter ton.AccountID, merkleRoot tlb.Bits256) (string, error) {
	data := JettonData{
		Status:              0,
		Balance:             0,
		OwnerAddress:        owner.ToMsgAddress(),
		JettonMasterAddress: minter.ToMsgAddress(),
		MerkleRoot:          merkleRoot,
	}

	dataCell := boc.NewCell()
	if err := tlb.Marshal(dataCell, data); err != nil {
		return "", err
	}
	jettonWalletCodeCells, err := boc.DeserializeBocHex("b5ee9c720101010100230008420259c02d4546e62393684b9ec55ae8b1c9d169415ff94502a93a63b0566c27ba15")
	if err != nil {
		return "", err
	}
	if len(jettonWalletCodeCells) != 1 {
		return "", fmt.Errorf("unexpected number of cells")
	}

	state := tlb.StateInit{
		Code: tlb.Maybe[tlb.Ref[boc.Cell]]{Exists: true, Value: tlb.Ref[boc.Cell]{Value: *jettonWalletCodeCells[0]}},
		Data: tlb.Maybe[tlb.Ref[boc.Cell]]{Exists: true, Value: tlb.Ref[boc.Cell]{Value: *dataCell}},
	}
	c := boc.NewCell()
	if err := tlb.Marshal(c, state); err != nil {
		return "", err
	}
	return c.ToBocBase64()
}
