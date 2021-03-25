package slasher

import (
	"context"
	"testing"

	ethpb "github.com/prysmaticlabs/ethereumapis/eth/v1alpha1"
	mock "github.com/prysmaticlabs/prysm/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/beacon-chain/core/helpers"
	dbtest "github.com/prysmaticlabs/prysm/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/beacon-chain/operations/slashings"
	"github.com/prysmaticlabs/prysm/beacon-chain/state/stategen"
	pb "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	"github.com/prysmaticlabs/prysm/shared/bls"
	"github.com/prysmaticlabs/prysm/shared/bytesutil"
	"github.com/prysmaticlabs/prysm/shared/params"
	"github.com/prysmaticlabs/prysm/shared/testutil"
	"github.com/prysmaticlabs/prysm/shared/testutil/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestService_processAttesterSlashings(t *testing.T) {
	ctx := context.Background()
	beaconDB := dbtest.SetupDB(t)

	beaconState, err := testutil.NewBeaconState()
	require.NoError(t, err)

	privKey, err := bls.RandKey()
	require.NoError(t, err)
	validators := make([]*ethpb.Validator, 1)
	validators[0] = &ethpb.Validator{
		PublicKey:             privKey.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 32),
		EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
	}
	err = beaconState.SetValidators(validators)
	require.NoError(t, err)

	mockChain := &mock.ChainService{
		State: beaconState,
	}
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:                beaconDB,
			AttestationStateFetcher: mockChain,
			StateGen:                stategen.New(beaconDB),
			SlashingPoolInserter:    &slashings.PoolMock{},
			HeadStateFetcher:        mockChain,
		},
	}

	firstAtt := testutil.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		AttestingIndices: []uint64{0},
	})
	secondAtt := testutil.HydrateIndexedAttestation(&ethpb.IndexedAttestation{
		AttestingIndices: []uint64{0},
	})

	domain, err := helpers.Domain(
		beaconState.Fork(),
		0,
		params.BeaconConfig().DomainBeaconAttester,
		beaconState.GenesisValidatorRoot(),
	)
	require.NoError(t, err)
	signingRoot, err := helpers.ComputeSigningRoot(firstAtt.Data, domain)
	require.NoError(t, err)

	t.Run("first_att_valid_sig_second_invalid", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use valid signature for the first att, but bad one for the second.
		signature := privKey.Sign(signingRoot[:])
		firstAtt.Signature = signature.Marshal()
		secondAtt.Signature = make([]byte, 96)

		slashings := []*ethpb.AttesterSlashing{
			{
				Attestation_1: firstAtt,
				Attestation_2: secondAtt,
			},
		}

		err = s.processAttesterSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsContain(tt, hook, "Invalid signature")
	})

	t.Run("first_att_invalid_sig_second_valid", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use invalid signature for the first att, but valid for the second.
		signature := privKey.Sign(signingRoot[:])
		firstAtt.Signature = make([]byte, 96)
		secondAtt.Signature = signature.Marshal()

		slashings := []*ethpb.AttesterSlashing{
			{
				Attestation_1: firstAtt,
				Attestation_2: secondAtt,
			},
		}

		err = s.processAttesterSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsContain(tt, hook, "Invalid signature")
	})

	t.Run("both_valid_att_signatures", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use valid signatures.
		signature := privKey.Sign(signingRoot[:])
		firstAtt.Signature = signature.Marshal()
		secondAtt.Signature = signature.Marshal()

		slashings := []*ethpb.AttesterSlashing{
			{
				Attestation_1: firstAtt,
				Attestation_2: secondAtt,
			},
		}

		err = s.processAttesterSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsDoNotContain(tt, hook, "Invalid signature")
	})
}

func TestService_processProposerSlashings(t *testing.T) {
	ctx := context.Background()
	beaconDB := dbtest.SetupDB(t)

	beaconState, err := testutil.NewBeaconState()
	require.NoError(t, err)

	privKey, err := bls.RandKey()
	require.NoError(t, err)
	validators := make([]*ethpb.Validator, 1)
	validators[0] = &ethpb.Validator{
		PublicKey:             privKey.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 32),
		EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
	}
	err = beaconState.SetValidators(validators)
	require.NoError(t, err)

	mockChain := &mock.ChainService{
		State: beaconState,
	}
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:                beaconDB,
			AttestationStateFetcher: mockChain,
			StateGen:                stategen.New(beaconDB),
			SlashingPoolInserter:    &slashings.PoolMock{},
			HeadStateFetcher:        mockChain,
		},
	}

	parentRoot := bytesutil.ToBytes32([]byte("parent"))
	err = s.serviceCfg.StateGen.SaveState(ctx, parentRoot, beaconState)
	require.NoError(t, err)

	firstBlockHeader := testutil.HydrateSignedBeaconHeader(&ethpb.SignedBeaconBlockHeader{
		Header: &ethpb.BeaconBlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    parentRoot[:],
		},
	})
	secondBlockHeader := testutil.HydrateSignedBeaconHeader(&ethpb.SignedBeaconBlockHeader{
		Header: &ethpb.BeaconBlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    parentRoot[:],
		},
	})

	domain, err := helpers.Domain(
		beaconState.Fork(),
		0,
		params.BeaconConfig().DomainBeaconProposer,
		beaconState.GenesisValidatorRoot(),
	)
	require.NoError(t, err)
	htr, err := firstBlockHeader.Header.HashTreeRoot()
	require.NoError(t, err)
	container := &pb.SigningData{
		ObjectRoot: htr[:],
		Domain:     domain,
	}
	require.NoError(t, err)
	signingRoot, err := container.HashTreeRoot()
	require.NoError(t, err)

	t.Run("first_header_valid_sig_second_invalid", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use valid signature for the first header, but bad one for the second.
		signature := privKey.Sign(signingRoot[:])
		firstBlockHeader.Signature = signature.Marshal()
		secondBlockHeader.Signature = make([]byte, 96)

		slashings := []*ethpb.ProposerSlashing{
			{
				Header_1: firstBlockHeader,
				Header_2: secondBlockHeader,
			},
		}

		err = s.processProposerSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsContain(tt, hook, "Invalid signature")
	})

	t.Run("first_header_invalid_sig_second_valid", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use invalid signature for the first header, but valid for the second.
		signature := privKey.Sign(signingRoot[:])
		firstBlockHeader.Signature = make([]byte, 96)
		secondBlockHeader.Signature = signature.Marshal()

		slashings := []*ethpb.ProposerSlashing{
			{
				Header_1: firstBlockHeader,
				Header_2: secondBlockHeader,
			},
		}

		err = s.processProposerSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsContain(tt, hook, "Invalid signature")
	})

	t.Run("both_valid_header_signatures", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use valid signatures.
		signature := privKey.Sign(signingRoot[:])
		firstBlockHeader.Signature = signature.Marshal()
		secondBlockHeader.Signature = signature.Marshal()

		slashings := []*ethpb.ProposerSlashing{
			{
				Header_1: firstBlockHeader,
				Header_2: secondBlockHeader,
			},
		}

		err = s.processProposerSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsDoNotContain(tt, hook, "Invalid signature")
	})
}