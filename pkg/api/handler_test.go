package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/liteapi"
	"github.com/tonkeeper/tongo/tlb"
	"github.com/tonkeeper/tongo/ton"
	"go.uber.org/zap"
)

func TestHandler_getStateInit(t *testing.T) {
	tests := []struct {
		name    string
		owner   ton.AccountID
		wantErr bool
	}{
		{
			owner: ton.MustParseAccountID("0:6ccd325a858c379693fae2bcaab1c2906831a4e10a6c3bb44ee8b615bca1d220"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := zap.NewDevelopment()
			cli, err := liteapi.NewClient(liteapi.Mainnet())
			require.Nil(t, err)
			h := &Handler{
				cli:          cli,
				logger:       logger,
				jettonMaster: ton.MustParseAccountID("EQD6Z9DHc5Mx-8PI8I4BjGX0d2NhapaRAK12CgstweNoMint"),
			}
			stateInit, err := h.getStateInit(context.Background(), tt.owner)
			require.Nil(t, err)
			cells, err := boc.DeserializeBocBase64(stateInit)
			require.Nil(t, err)
			require.Equal(t, 1, len(cells))

			var init tlb.StateInit
			require.Nil(t, tlb.Unmarshal(cells[0], &init))
			fmt.Printf("stateInit: %v\n", stateInit)
		})
	}
}
