package prover

import (
	"context"
	"fmt"
	"os"

	"github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/tlb"
	"github.com/tonkeeper/tongo/ton"
	"go.uber.org/zap"

	"github.com/tonkeeper/claim-api-go/pkg/utils"
)

type ProofResponse struct {
	Proof          []byte
	AirdropPayload AirdropData
	Err            error
}

type ProofRequest struct {
	AccountID  ton.AccountID
	ResponseCh chan<- ProofResponse
}

type EnumerateResponse struct {
	Airdrop  []WalletAirdrop
	NextFrom ton.AccountID
	Err      error
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

func (p *Prover) Run(ctx context.Context) {
	go p.queue.Run(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-p.queue.Output():
			switch x := req.(type) {
			case ProofRequest:
				data, proof, err := prove(x.AccountID, p.merkleProver, p.root)
				if err != nil {
					x.ResponseCh <- ProofResponse{
						Err: err,
					}
					continue
				}
				x.ResponseCh <- ProofResponse{
					Proof:          proof,
					AirdropPayload: data,
				}
			case EnumerateRequest:
				accounts, err := enumerateAccounts(x.NextFrom, p.root, x.Count+1)
				if err != nil {
					x.ResponseCh <- EnumerateResponse{
						Err: err,
					}
					continue
				}
				allGood := true
				airdrop := make([]WalletAirdrop, 0, len(accounts))
				var nextFrom ton.AccountID
				if len(accounts) == x.Count+1 {
					nextFrom = accounts[len(accounts)-1]
					accounts = accounts[:len(accounts)-1]
				}
				for _, accountID := range accounts {
					data, proof, err := prove(accountID, p.merkleProver, p.root)
					if err != nil {
						x.ResponseCh <- EnumerateResponse{
							Err: err,
						}
						allGood = false
						break
					}
					airdrop = append(airdrop, WalletAirdrop{
						AccountID: accountID,
						Data:      data,
						Proof:     proof,
					})
				}
				if allGood {
					x.ResponseCh <- EnumerateResponse{
						Airdrop:  airdrop,
						NextFrom: nextFrom,
					}
				}
			default:
				p.logger.Error("unexpected request type", zap.Any("req", req))
			}
		}
	}
}

func (p *Prover) Queue() chan<- any {
	return p.queue.Input()
}

func (p *Prover) MerkleRoot() tlb.Bits256 {
	return p.merkleRoot
}

func prove(accountID ton.AccountID, prover *boc.MerkleProver, root *boc.Cell) (AirdropData, []byte, error) {
	msgAddr := accountID.ToMsgAddress()
	addrCell := boc.NewCell()
	if err := tlb.Marshal(addrCell, msgAddr); err != nil {
		return AirdropData{}, nil, err
	}
	addrCell.ResetCounters()
	root.ResetCounters()
	return tlb.ProveKeyInHashmap[AirdropData](prover, root, addrCell.ReadRemainingBits())
}

func enumerateAccounts(nextFrom ton.AccountID, root *boc.Cell, count int) ([]ton.AccountID, error) {
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
