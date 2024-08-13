package prover

import (
	"context"
	"fmt"
	"os"

	"github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/tlb"
	"go.uber.org/zap"

	"github.com/tonkeeper/claim-api-go/pkg/utils"
)

type ProofResponse struct {
	Proof          []byte
	AirdropPayload *AirdropPayload
	Err            error
}

type ProofRequest struct {
	AccountID  *boc.Cell
	ResponseCh chan ProofResponse
}

type Prover struct {
	root         *boc.Cell
	queue        *utils.ElasticQueue[ProofRequest]
	merkleProver *boc.MerkleProver
}

type Config struct {
	Filename string
}

type AirdropPayload struct {
	Amount    tlb.Coins
	StartFrom tlb.Uint48
	ExpireAt  tlb.Uint48
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
	merkleProver, err := boc.NewMerkleProver(airdropCells[0])
	if err != nil {
		return nil, fmt.Errorf("failed to create merkle prover: %w", err)
	}
	return &Prover{
		root:         airdropCells[0],
		merkleProver: merkleProver,
		queue:        utils.NewQueue[ProofRequest]("prover", utils.WithMaxLength(1000)),
	}, nil
}

func (p *Prover) Run(ctx context.Context) {
	go p.queue.Run(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-p.queue.Output():
			data, proof, err := tlb.ProveKeyInHashmap[AirdropPayload](p.merkleProver, p.root, req.AccountID.ReadRemainingBits())
			if err != nil {
				req.ResponseCh <- ProofResponse{
					Err: err,
				}
			} else {
				req.ResponseCh <- ProofResponse{
					Proof:          proof,
					AirdropPayload: &data,
				}
			}
		}
	}
}

func (p *Prover) Queue() chan<- ProofRequest {
	return p.queue.Input()
}
