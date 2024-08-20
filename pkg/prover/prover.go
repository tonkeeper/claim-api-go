package prover

import (
	"context"
	"fmt"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/tlb"
	"github.com/tonkeeper/tongo/ton"
	"go.uber.org/zap"

	"github.com/tonkeeper/claim-api-go/pkg/utils"
)

type ProofResponse struct {
	WalletAirdrop WalletAirdrop
	Err           error
}

type ProofRequest struct {
	AccountID  ton.AccountID
	ResponseCh chan<- ProofResponse
}

type EnumerateResponse struct {
	WalletAirdrops []WalletAirdrop
	NextFrom       ton.AccountID
	Err            error
}

type EnumerateRequest struct {
	NextFrom   ton.AccountID
	Count      int
	ResponseCh chan<- EnumerateResponse
}

type Prover struct {
	logger       *zap.Logger
	root         *boc.Cell
	queue        *utils.ElasticQueue[any]
	merkleProver *boc.MerkleProver
	merkleRoot   tlb.Bits256
}

type Config struct {
	Filename string
}

type AirdropData struct {
	Amount    tlb.Coins
	StartFrom tlb.Uint48
	ExpireAt  tlb.Uint48
}

type WalletAirdrop struct {
	AccountID ton.AccountID
	Data      AirdropData
	Proof     []byte
}

func NewProver(logger *zap.Logger, conf Config) (*Prover, error) {
	content, err := os.ReadFile(conf.Filename)
	if err != nil {
		return nil, err
	}
	airdropCells, err := boc.DeserializeBoc(content)
	if err != nil {
		return nil, err
	}
	if len(airdropCells) != 1 {
		return nil, fmt.Errorf("invalid airdrop data, got number of root cells: %v", len(airdropCells))
	}
	merkleRoot, err := airdropCells[0].Hash()
	if err != nil {
		return nil, fmt.Errorf("failed to calculate merkle root: %w", err)
	}
	airdropCells[0].ResetCounters()
	merkleProver, err := boc.NewMerkleProver(airdropCells[0])
	if err != nil {
		return nil, fmt.Errorf("failed to create merkle prover: %w", err)
	}
	return &Prover{
		logger:       logger,
		root:         airdropCells[0],
		merkleProver: merkleProver,
		merkleRoot:   tlb.Bits256(merkleRoot),
		queue:        utils.NewQueue[any]("prover", utils.WithMaxLength(1000)),
	}, nil
}

func (p *Prover) Queue() chan<- any {
	return p.queue.Input()
}

func (p *Prover) MerkleRoot() tlb.Bits256 {
	return p.merkleRoot
}

func (p *Prover) Run(ctx context.Context) {
	go p.queue.Run(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case reqAny := <-p.queue.Output():
			switch req := reqAny.(type) {
			case ProofRequest:
				p.processProofRequest(req)
			case EnumerateRequest:
				p.processEnumerateAccountsRequest(req)
			default:
				p.logger.Error("unexpected request type", zap.Any("reqAny", reqAny))
			}
		}
	}
}

func (p *Prover) processProofRequest(req ProofRequest) {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		proverTimeHistogramVec.WithLabelValues("processProofRequest").Observe(v)
	}))
	defer timer.ObserveDuration()

	walletAirdrop, err := prove(req.AccountID, p.merkleProver, p.root)
	if err != nil {
		req.ResponseCh <- ProofResponse{
			Err: err,
		}
		return
	}
	req.ResponseCh <- ProofResponse{
		WalletAirdrop: walletAirdrop,
	}
}

func (p *Prover) processEnumerateAccountsRequest(req EnumerateRequest) {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		proverTimeHistogramVec.WithLabelValues("processEnumerateAccountsRequest").Observe(v)
	}))
	defer timer.ObserveDuration()

	walledDatas, err := enumerateAccounts(req.NextFrom, p.root, req.Count+1)
	if err != nil {
		req.ResponseCh <- EnumerateResponse{
			Err: err,
		}
		return
	}
	var nextFrom ton.AccountID
	if len(walledDatas) == req.Count+1 {
		nextFrom = walledDatas[len(walledDatas)-1].AccountID
		walledDatas = walledDatas[:len(walledDatas)-1]
	}
	airdrop := make([]WalletAirdrop, 0, len(walledDatas))
	for _, data := range walledDatas {
		airdrop = append(airdrop, WalletAirdrop{
			AccountID: data.AccountID,
			Data:      data.Data,
		})
	}
	req.ResponseCh <- EnumerateResponse{
		WalletAirdrops: airdrop,
		NextFrom:       nextFrom,
	}
}

func prove(accountID ton.AccountID, prover *boc.MerkleProver, root *boc.Cell) (WalletAirdrop, error) {
	msgAddr := accountID.ToMsgAddress()
	addrCell := boc.NewCell()
	if err := tlb.Marshal(addrCell, msgAddr); err != nil {
		return WalletAirdrop{}, err
	}
	addrCell.ResetCounters()
	root.ResetCounters()
	data, proof, err := tlb.ProveKeyInHashmap[AirdropData](prover, root, addrCell.ReadRemainingBits())
	if err != nil {
		return WalletAirdrop{}, err
	}
	return WalletAirdrop{
		AccountID: accountID,
		Data:      data,
		Proof:     proof,
	}, nil
}

func enumerateAccounts(nextFrom ton.AccountID, root *boc.Cell, count int) ([]walletData, error) {
	root.ResetCounters()
	prefix := boc.NewBitString(0)
	startKey, err := accountIDToBitString(nextFrom)
	if err != nil {
		return nil, err
	}
	return walk(startKey, &prefix, root, count)
}

func accountIDToBitString(accountID ton.AccountID) (*boc.BitString, error) {
	msgAddr := accountID.ToMsgAddress()
	c := boc.NewCell()
	if err := tlb.Marshal(c, msgAddr); err != nil {
		return nil, err
	}
	c.ResetCounters()
	bitString := c.ReadRemainingBits()
	return &bitString, nil
}
