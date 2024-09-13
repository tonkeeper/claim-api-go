package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/tonkeeper/tongo/abi"
	boc "github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/contract/jetton"
	"github.com/tonkeeper/tongo/liteapi"
	"github.com/tonkeeper/tongo/tlb"
	"github.com/tonkeeper/tongo/ton"
	"go.uber.org/zap"

	"github.com/tonkeeper/claim-api-go/pkg/api/oas"
	"github.com/tonkeeper/claim-api-go/pkg/prover"
)

// Handler handles operations described by OpenAPI v3 specification of this service.
// It implements oas.Handler interface and every API operation is implemented as a method on Handler.
type Handler struct {
	logger *zap.Logger

	prover       *prover.Prover
	jettonMaster ton.AccountID
	cli          *liteapi.Client
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
		cli:          cli,
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

func (h *Handler) convertToWalletInfo(ctx context.Context, airdrop prover.WalletAirdrop) (*oas.WalletInfo, error) {
	customPayload, err := createCustomPayload(airdrop.Proof)
	if err != nil {
		return nil, err
	}
	stateInit, err := h.getStateInit(ctx, airdrop.AccountID)
	if err != nil {
		return nil, err
	}
	compressedInfo := oas.WalletInfoCompressedInfo{
		Amount:    strconv.FormatUint(uint64(airdrop.Data.Amount), 10),
		StartFrom: strconv.FormatUint(uint64(airdrop.Data.StartFrom), 10),
		ExpiredAt: strconv.FormatUint(uint64(airdrop.Data.ExpireAt), 10),
	}
	return &oas.WalletInfo{
		Owner:          airdrop.AccountID.ToRaw(),
		CustomPayload:  customPayload,
		StateInit:      oas.NewOptString(stateInit),
		CompressedInfo: oas.NewOptWalletInfoCompressedInfo(compressedInfo),
	}, nil
}

func (h *Handler) GetWalletInfo(ctx context.Context, params oas.GetWalletInfoParams) (*oas.WalletInfo, error) {
	accountID, err := ton.ParseAccountID(params.Address)
	if err != nil {
		return nil, BadRequest("failed to parse account id")
	}
	responseCh := make(chan prover.ProofResponse, 1)
	h.prover.Queue() <- prover.ProofRequest{
		AccountID:  accountID,
		ResponseCh: responseCh,
	}
	select {
	case <-ctx.Done():
		return nil, BadRequest("timeout")
	case resp := <-responseCh:
		if resp.Err != nil && strings.Contains(resp.Err.Error(), "key is not found") {
			return nil, NotFound("account not not found")
		}
		if resp.Err != nil {
			return nil, InternalError(resp.Err)
		}
		info, err := h.convertToWalletInfo(ctx, resp.WalletAirdrop)
		if err != nil {
			return nil, InternalError(err)
		}
		return info, nil
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

func (h *Handler) GetWallets(ctx context.Context, params oas.GetWalletsParams) (*oas.WalletList, error) {
	next, err := ton.ParseAccountID(params.NextFrom)
	if err != nil {
		return nil, BadRequest("failed to parse next from")
	}
	ch := make(chan prover.EnumerateResponse, 1)
	h.prover.Queue() <- prover.EnumerateRequest{
		NextFrom:   next,
		Count:      params.Count,
		ResponseCh: ch,
	}
	select {
	case <-ctx.Done():
		return nil, BadRequest("timeout")
	case resp := <-ch:
		if resp.Err != nil && strings.Contains(resp.Err.Error(), "key is not found") {
			return nil, NotFound("account not not found")
		}
		if resp.Err != nil {
			return nil, InternalError(resp.Err)
		}
		items := make([]oas.WalletListWalletsItem, 0, len(resp.WalletAirdrops))
		for _, walletAirdrop := range resp.WalletAirdrops {
			item := oas.WalletListWalletsItem{
				Owner: walletAirdrop.AccountID.ToRaw(),
				CompressedInfo: oas.WalletListWalletsItemCompressedInfo{
					Amount:    strconv.FormatUint(uint64(walletAirdrop.Data.Amount), 10),
					StartFrom: strconv.FormatUint(uint64(walletAirdrop.Data.StartFrom), 10),
					ExpiredAt: strconv.FormatUint(uint64(walletAirdrop.Data.ExpireAt), 10),
				},
			}
			items = append(items, item)
		}
		var nextFrom string
		if !resp.NextFrom.IsZero() {
			nextFrom = resp.NextFrom.ToRaw()
		}
		return &oas.WalletList{Wallets: items, NextFrom: nextFrom}, nil
	}
}

func (h *Handler) getStateInit(ctx context.Context, owner ton.AccountID) (string, error) {
	// TODO: add cache
	var stateInit boc.Cell
	err := retry.Do(func() error {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_, value, err := GetWalletStateInitAndSalt(ctx, h.cli, h.jettonMaster, owner.ToMsgAddress())
		if err != nil {
			return err
		}
		result, ok := value.(GetWalletStateInitAndSaltResult)
		if !ok {
			return fmt.Errorf("failed to get state init")
		}
		stateInit = boc.Cell(result.StateInit)
		return nil
	}, retry.Attempts(3), retry.Delay(1*time.Second))
	if err != nil {
		return "", err
	}
	return stateInit.ToBocBase64()
}

type GetWalletStateInitAndSaltResult struct {
	StateInit tlb.Any
	Salt      int64
}

func GetWalletStateInitAndSalt(ctx context.Context, executor abi.Executor, reqAccountID ton.AccountID, ownerAddress tlb.MsgAddress) (string, any, error) {
	stack := tlb.VmStack{}
	var (
		val tlb.VmStackValue
		err error
	)
	val, err = tlb.TlbStructToVmCellSlice(ownerAddress)
	if err != nil {
		return "", nil, err
	}
	stack.Put(val)

	// MethodID = 69258 for "get_wallet_state_init_and_salt" method
	errCode, stack, err := executor.RunSmcMethodByID(ctx, reqAccountID, 69258, stack)
	if err != nil {
		return "", nil, err
	}
	if errCode != 0 && errCode != 1 {
		return "", nil, fmt.Errorf("method execution failed with code: %v", errCode)
	}
	for _, f := range []func(tlb.VmStack) (string, any, error){DecodeGetWalletStateInitAndSaltResult} {
		s, r, err := f(stack)
		if err == nil {
			return s, r, nil
		}
	}
	return "", nil, fmt.Errorf("can not decode outputs")
}

func DecodeGetWalletStateInitAndSaltResult(stack tlb.VmStack) (resultType string, resultAny any, err error) {
	if len(stack) != 2 || (stack[0].SumType != "VmStkCell") || (stack[1].SumType != "VmStkTinyInt" && stack[1].SumType != "VmStkInt") {
		return "", nil, fmt.Errorf("invalid stack format")
	}
	var result GetWalletStateInitAndSaltResult
	err = stack.Unmarshal(&result)
	return "GetWalletStateInitAndSaltResult", result, err
}
