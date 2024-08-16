package prover

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tonkeeper/tongo"
	"github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/tlb"
	"github.com/tonkeeper/tongo/ton"
)

type Address struct {
	tlb.MsgAddress
}

func (addr Address) Equal(other any) bool {
	otherAddr, ok := other.(Address)
	if !ok {
		return false
	}
	return addr.MsgAddress == otherAddr.MsgAddress
}

func (addr Address) FixedSize() int {
	return 267
}

func (addr *Address) UnmarshalTLB(c *boc.Cell, decoder *tlb.Decoder) error {
	var msgAddr tlb.MsgAddress
	if err := decoder.Unmarshal(c, &msgAddr); err != nil {
		return err
	}

	*addr = Address{MsgAddress: msgAddr}
	return nil
}
func (addr *Address) ToRaw() string {
	account, err := tongo.AccountIDFromTlb(addr.MsgAddress)
	if err != nil {
		panic(err)
	}
	return account.String()
}

func readAirdropDataFile(t *testing.T, filename string) (*boc.Cell, tlb.Hashmap[Address, AirdropData]) {
	content, err := os.ReadFile("testdata/airdropData.boc")
	require.Nil(t, err)
	airdropCells, err := boc.DeserializeBoc(content)
	require.Nil(t, err)
	require.Equal(t, 1, len(airdropCells))
	root := airdropCells[0]

	var hashmap tlb.Hashmap[Address, AirdropData]
	err = tlb.Unmarshal(root, &hashmap)
	require.Nil(t, err)
	return root, hashmap
}

func Test_enumerateAccounts(t *testing.T) {
	root, hashmap := readAirdropDataFile(t, "testdata/airdropData.boc")
	var all []string
	for _, key := range hashmap.Keys() {
		all = append(all, key.ToRaw())
	}

	tests := []struct {
		name     string
		nextFrom ton.AccountID
		count    int
		want     []string
		wantErr  bool
	}{
		{
			name:  "first 10",
			count: 5,
			want: []string{
				"0:004bbd06fb606418d6e83916ee891845451335c3883b3aa6f502e24c5f5b1985",
				"0:00fdb15f679957128fd0ee8f740aaca4f37a6877e31d61a454ed9c7604a5c1dc",
				"0:02b17a8849e3ba23ccdecbba4fd1b57da7031b65119e6cc371ef393d913bf410",
				"0:03e71d0515f2a6e4df555954f05b0e67859298dfe8a13066c35ac7f2eed93991",
				"0:050b89727f74efd71e3f5c396c76c6df7ee71aced7c2ec7a8c55bb8bba8d1399",
			},
		},
		{
			name:     "skip first 2",
			count:    5,
			nextFrom: ton.MustParseAccountID("0:00fdb15f679957128fd0ee8f740aaca4f37a6877e31d61a454ed9c7604a5c2dc"),
			want: []string{
				"0:02b17a8849e3ba23ccdecbba4fd1b57da7031b65119e6cc371ef393d913bf410",
				"0:03e71d0515f2a6e4df555954f05b0e67859298dfe8a13066c35ac7f2eed93991",
				"0:050b89727f74efd71e3f5c396c76c6df7ee71aced7c2ec7a8c55bb8bba8d1399",
				"0:05229d383a97f30a1bfbc8f35d9e62b0cca908f915498b1e88a97a86bfb44d95",
				"0:0599b95c20154ee8ec7b546bfe1fa1252f3d2299168fa2952df3967970dae221",
			},
		},
		{
			name:     "last 4",
			count:    5,
			nextFrom: ton.MustParseAccountID("0:fdde373e334c8a72e19c38b0becd9c047f10846aeac53c9a410738d78c21d450"),
			want: []string{
				"0:fdde373e334c8a72e19c38b0becd9c047f10846aeac53c9a410738d78c21d450",
				"0:fdde49b1562a3ee30e8755d903d2fe1b199b57503f2e55dc897f613685d821d7",
				"0:fed8483b9d943ddd3c816170317663c1bc0e4de644e9b141b8dcc0453fc9d232",
				"0:ff41b315c634b4ea4814b9262499567d36e9c7b13da09476f11a41d94e2cb7ff",
			},
		},
		{
			name:  "all",
			count: 1000,
			want:  all,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root.ResetCounters()
			accounts, err := enumerateAccounts(tt.nextFrom, root, tt.count)
			require.Nil(t, err)
			accs := make([]string, 0, len(accounts))
			for _, account := range accounts {
				accs = append(accs, account.ToRaw())
			}
			require.Equal(t, tt.want, accs)
		})
	}
}

func Test_prove(t *testing.T) {
	root, hashmap := readAirdropDataFile(t, "testdata/airdropData.boc")
	tests := []struct {
		name      string
		accountID ton.AccountID
		wantErr   string
	}{
		{
			name:      "all good",
			accountID: ton.MustParseAccountID("0:050b89727f74efd71e3f5c396c76c6df7ee71aced7c2ec7a8c55bb8bba8d1399"),
		},
		{
			name:      "last one",
			accountID: ton.MustParseAccountID("0:ff41b315c634b4ea4814b9262499567d36e9c7b13da09476f11a41d94e2cb7ff"),
		},
		{
			name:      "absent",
			accountID: ton.MustParseAccountID("0:ff41b315c634b4ea4814b9262499567d36e9c7b13da09476f11a41d94e2cb700"),
			wantErr:   "key is not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root.ResetCounters()
			prover, err := boc.NewMerkleProver(root)
			require.Nil(t, err)
			walletAirdrop, err := prove(tt.accountID, prover, root)
			if len(tt.wantErr) > 0 {
				require.NotNil(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.Nil(t, err)
			require.Equal(t, tt.accountID, walletAirdrop.AccountID)
			data, ok := hashmap.Get(Address{MsgAddress: tt.accountID.ToMsgAddress()})
			require.True(t, ok)
			require.Equal(t, data, walletAirdrop.Data)

			airdropCells, err := boc.DeserializeBoc(walletAirdrop.Proof)
			require.Nil(t, err)
			require.Equal(t, 1, len(airdropCells))
			proofRoot := airdropCells[0]

			ref, err := proofRoot.NextRef()
			require.Nil(t, err)

			var hashmapFromProof tlb.Hashmap[Address, AirdropData]
			err = tlb.Unmarshal(ref, &hashmapFromProof)
			require.Nil(t, err)

			dataInProof, ok := hashmapFromProof.Get(Address{MsgAddress: tt.accountID.ToMsgAddress()})
			require.True(t, ok)

			require.Equal(t, dataInProof, walletAirdrop.Data)

		})
	}
}
