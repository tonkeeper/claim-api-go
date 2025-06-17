package prover

import (
	"fmt"
	"github.com/tonkeeper/tongo/boc"
	"os"
	"testing"
)

func getAirdropRoot() (*boc.Cell, error) {
	content, err := os.ReadFile("testdata/airdropData.boc")
	if err != nil {
		return nil, err
	}
	airdropCells, err := boc.DeserializeBoc(content)
	if err != nil {
		return nil, err
	}
	if len(airdropCells) != 1 {
		return nil, fmt.Errorf("incorrect number of roots")
	}
	root := airdropCells[0]

	return root, nil
}

func Test_countLeaves(t *testing.T) {
	root, err := getAirdropRoot()
	if err != nil {
		t.Fatal(err)
	}
	prefix := boc.NewBitString(0)
	cnt, err := countLeaves(&prefix, root)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 360 {
		t.Errorf("incorrect number of leaves: got %v, want %v", cnt, 360)
	}
}
