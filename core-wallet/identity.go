package main

import (
	"context"
	"github.com/iden3/go-circuits"
	core "github.com/iden3/go-iden3-core"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/iden3/go-merkletree-sql/v2"
	"github.com/iden3/go-merkletree-sql/v2/db/memory"
	"log"
	"math/big"
)

type Identity struct {
	PrivateKey babyjub.PrivateKey
	ID         *core.ID
	Clt        *merkletree.MerkleTree
	Ret        *merkletree.MerkleTree
	Rot        *merkletree.MerkleTree
	AuthClaim  *core.Claim
}

func NewIdentity(privateKey babyjub.PrivateKey, ID *core.ID, clt *merkletree.MerkleTree, ret *merkletree.MerkleTree, rot *merkletree.MerkleTree, authClaim *core.Claim) *Identity {
	return &Identity{PrivateKey: privateKey, ID: ID, Clt: clt, Ret: ret, Rot: rot, AuthClaim: authClaim}
}

func FromFileData(account *Account) *Identity {
	ctx := context.Background()
	clt, _ := merkletree.NewMerkleTree(ctx, memory.NewMemoryStorage(), 32)
	ret, _ := merkletree.NewMerkleTree(ctx, memory.NewMemoryStorage(), 32)
	rot, _ := merkletree.NewMerkleTree(ctx, memory.NewMemoryStorage(), 32)

	hIndex, hValue, _ := account.AuthClaim.HiHv()

	clt.Add(ctx, hIndex, hValue)

	state, _ := merkletree.HashElems(
		clt.Root().BigInt(),
		ret.Root().BigInt(),
		rot.Root().BigInt())

	id, _ := core.IdGenesisFromIdenState(core.TypeDefault, state.BigInt())

	if id.String() != account.ID.String() {
		log.Fatal("ID differs while recreating it from json file.", id, account.ID)
	}

	for i := 0; i < len(account.Claims); i++ {
		hIndex, hValue, _ := account.Claims[i].HiHv()
		clt.Add(ctx, hIndex, hValue)
	}

	state, _ = merkletree.HashElems(
		clt.Root().BigInt(),
		ret.Root().BigInt(),
		rot.Root().BigInt())

	if state.String() != account.IDS.String() {
		log.Fatal("IDS differs while recreating it from json file.")
	}

	return NewIdentity(account.PrivateKey, account.ID, clt, ret, rot, account.AuthClaim)
}

func (identity *Identity) addClaim(claim ClaimAPI) ([]byte, *core.Claim) {
	ctx := context.Background()

	authClaim := identity.AuthClaim
	authClaimIndex, _ := authClaim.HIndex()
	authClaimRevNonce := authClaim.GetRevocationNonce()

	// 1. Generate Merkle Tree Proof for authClaim at Genesis State
	authMTPProof, _, _ := identity.Clt.GenerateProof(ctx, authClaimIndex, identity.Clt.Root())

	// 2. Generate the Non-Revocation Merkle tree proof for the authClaim at Genesis State
	authNonRevMTPProof, _, _ := identity.Ret.GenerateProof(ctx, big.NewInt(int64(authClaimRevNonce)), identity.Ret.Root())

	oldState := identity.GetIDS()
	isOldStateGenesis := identity.IsAtGenesisState()
	oldTreeState := circuits.TreeState{
		State:          oldState,
		ClaimsRoot:     identity.Clt.Root(),
		RevocationRoot: identity.Ret.Root(),
		RootOfRoots:    identity.Rot.Root(),
	}

	// Before updating the claims tree, add the claims tree root at Genesis state to the Roots tree.
	identity.Rot.Add(ctx, identity.Clt.Root().BigInt(), big.NewInt(0))

	claimToAdd := createIden3ClaimFromAPI(claim)
	hIndex, hValue, _ := claimToAdd.HiHv()

	identity.Clt.Add(ctx, hIndex, hValue)
	// Fetch the new Identity State
	newState := identity.GetIDS()

	// Sign a message (hash of the genesis state + the new state) using your private key
	hashOldAndNewStates, _ := poseidon.Hash([]*big.Int{oldState.BigInt(), newState.BigInt()})

	signature := identity.PrivateKey.SignPoseidon(hashOldAndNewStates)

	// Generate state transition inputs
	stateTransitionInputs := circuits.StateTransitionInputs{
		ID:                identity.ID,
		OldTreeState:      oldTreeState,
		NewState:          newState,
		IsOldStateGenesis: isOldStateGenesis,
		AuthClaim: circuits.Claim{
			Claim: authClaim,
			Proof: authMTPProof,
			NonRevProof: &circuits.ClaimNonRevStatus{
				Proof: authNonRevMTPProof,
			},
		},
		Signature: signature,
	}

	// Perform marshalling of the state transition inputs
	inputBytes, _ := stateTransitionInputs.InputsMarshal()
	return inputBytes, claimToAdd
}

func (identity *Identity) GetIDS() *merkletree.Hash {
	state, _ := merkletree.HashElems(
		identity.Clt.Root().BigInt(),
		identity.Ret.Root().BigInt(),
		identity.Rot.Root().BigInt())
	return state
}

// IsAtGenesisState TODO: Implement this function
func (identity *Identity) IsAtGenesisState() bool {
	return true
}