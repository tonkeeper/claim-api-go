package prover

import (
	"github.com/tonkeeper/tongo"
	"github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/tlb"
	"github.com/tonkeeper/tongo/ton"
)

func readCommonPrefix(size int, c *boc.Cell) (int, *boc.BitString, error) {
	first, err := c.ReadBit()
	if err != nil {
		return 0, nil, err
	}
	// hml_short$0
	if !first {
		// Unary, while 1, add to ln
		ln, err := c.ReadUnary()
		if err != nil {
			return 0, nil, err
		}
		// add bits to key
		bitString := boc.NewBitString(int(ln))
		for i := 0; i < int(ln); i++ {
			bit, err := c.ReadBit()
			if err != nil {
				return 0, nil, err
			}
			if err := bitString.WriteBit(bit); err != nil {
				return 0, nil, err
			}
		}
		return int(ln), &bitString, nil
	}
	second, err := c.ReadBit()
	if err != nil {
		return 0, nil, err
	}
	// hml_long$10
	if !second {
		ln, err := c.ReadLimUint(size)
		if err != nil {
			return 0, nil, err
		}
		bitString := boc.NewBitString(int(ln))
		for i := 0; i < int(ln); i++ {
			bit, err := c.ReadBit()
			if err != nil {
				return 0, nil, err
			}
			if err := bitString.WriteBit(bit); err != nil {
				return 0, nil, err
			}
		}
		return int(ln), &bitString, nil
	}
	// hml_same$11
	bitType, err := c.ReadBit()
	if err != nil {
		return 0, nil, err
	}
	ln, err := c.ReadLimUint(size)
	if err != nil {
		return 0, nil, err
	}
	bitString := boc.NewBitString(int(ln))
	for i := 0; i < int(ln); i++ {
		if err := bitString.WriteBit(bitType); err != nil {
			return 0, nil, err
		}
	}
	return int(ln), &bitString, nil
}

// compareBitStrings returns
// 0 - key starts with prefix
// 1 - key > prefix
// -1 - key < prefix
func compareBitStrings(key *boc.BitString, prefix *boc.BitString) (int, error) {
	key.ResetCounter()
	prefix.ResetCounter()
	for prefix.BitsAvailableForRead() > 0 {
		prefixBit, err := prefix.ReadBit()
		if err != nil {
			panic(err)
		}
		keyBit, err := key.ReadBit()
		if err != nil {
			return 0, err
		}
		if prefixBit == keyBit {
			continue
		}
		if keyBit == false && prefixBit == true {
			return -1, nil
		}
		return 1, nil
	}
	// equal
	return 0, nil
}

func addBit(a *boc.BitString, bit bool) (*boc.BitString, error) {
	a.ResetCounter()
	b := boc.NewBitString(a.BitsAvailableForRead() + 1)
	for a.BitsAvailableForRead() > 0 {
		bit, err := a.ReadBit()
		if err != nil {
			return nil, err
		}
		if err := b.WriteBit(bit); err != nil {
			return nil, err
		}
	}
	if err := b.WriteBit(bit); err != nil {
		return nil, err
	}
	return &b, nil
}

func concatBitStrings(a, b *boc.BitString) (*boc.BitString, error) {
	a.ResetCounter()
	b.ResetCounter()
	c := boc.NewBitString(a.BitsAvailableForRead() + b.BitsAvailableForRead())
	for a.BitsAvailableForRead() > 0 {
		bit, err := a.ReadBit()
		if err != nil {
			return nil, err
		}
		if err := c.WriteBit(bit); err != nil {
			return nil, err
		}
	}
	for b.BitsAvailableForRead() > 0 {
		bit, err := b.ReadBit()
		if err != nil {
			return nil, err
		}
		if err := c.WriteBit(bit); err != nil {
			return nil, err
		}
	}
	return &c, nil
}

func bitsToAccountID(bitString *boc.BitString) (ton.AccountID, error) {
	bitString.ResetCounter()
	cell := boc.NewCellWithBits(*bitString)
	var addr tlb.MsgAddress
	if err := tlb.Unmarshal(cell, &addr); err != nil {
		return ton.AccountID{}, err
	}
	accountID, err := tongo.AccountIDFromTlb(addr)
	if err != nil {
		return ton.AccountID{}, err
	}
	return *accountID, nil
}

type walletData struct {
	AccountID ton.AccountID
	Data      AirdropData
}

func walk(startKey *boc.BitString, prefix *boc.BitString, cell *boc.Cell, count int) ([]walletData, error) {
	startKey.ResetCounter()
	prefix.ResetCounter()
	size := startKey.BitsAvailableForRead() - prefix.BitsAvailableForRead()
	prefixSize, nextPrefix, err := readCommonPrefix(size, cell)
	if err != nil {
		return nil, err
	}
	currentPrefix, err := concatBitStrings(prefix, nextPrefix)
	if err != nil {
		return nil, err
	}
	if size == prefixSize {
		c, err := compareBitStrings(startKey, currentPrefix)
		if err != nil {
			return nil, err
		}
		if c == 1 {
			// key > prefix and we have to skip this wallet.
			return nil, nil
		}
		accountID, err := bitsToAccountID(currentPrefix)
		if err != nil {
			return nil, err
		}
		var data AirdropData
		if err := tlb.Unmarshal(cell, &data); err != nil {
			return nil, err
		}
		return []walletData{{AccountID: accountID, Data: data}}, nil
	}
	c, err := compareBitStrings(startKey, currentPrefix)
	if err != nil {
		return nil, err
	}
	skipLeft := false
	switch c {
	case -1:
		// key < prefix
	case 0:
		startKey.ResetCounter()
		currentPrefix.ResetCounter()
		if err := startKey.Skip(currentPrefix.BitsAvailableForRead()); err != nil {
			return nil, err
		}
		isRight, err := startKey.ReadBit()
		if err != nil {
			return nil, err
		}
		if isRight {
			skipLeft = true
		}
	case 1:
		return nil, nil
	}
	var arrLeft []walletData
	if skipLeft {
		_, err := cell.NextRef()
		if err != nil {
			return nil, err
		}
	} else {
		left, err := addBit(currentPrefix, false)
		if err != nil {
			return nil, err
		}
		leftRef, err := cell.NextRef()
		if err != nil {
			return nil, err
		}
		arrLeft, err = walk(startKey, left, leftRef, count)
		if err != nil {
			return nil, err
		}
		if len(arrLeft) == count {
			return arrLeft, nil
		}
	}
	right, err := addBit(currentPrefix, true)
	if err != nil {
		return nil, err
	}
	rightRef, err := cell.NextRef()
	if err != nil {
		return nil, err
	}
	arrRight, err := walk(startKey, right, rightRef, count-len(arrLeft))
	if err != nil {
		return nil, err
	}
	return append(arrLeft, arrRight...), nil
}
