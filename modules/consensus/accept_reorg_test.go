package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// reorgSets contains multiple consensus sets that share a genesis block, which
// can be manipulated to cause full integration blockchain reorgs.
//
// cstBackup is a holding place for cstMain - the blocks originally in cstMain get moved
// to cstBackup so that cstMain can be reorganized without that history being lost.
// Extending cstBackup will allow cstMain to be reorg'd back to its original blocks.
type reorgSets struct {
	cstMain   *consensusSetTester
	cstAlt    *consensusSetTester
	cstBackup *consensusSetTester
}

// createReorgSets creates a reorg set that is ready to be manipulated.
func createReorgSets(name string) *reorgSets {
	cstMain, err := createConsensusSetTester(name + " - 1")
	if err != nil {
		panic(err)
	}
	defer cstMain.closeCst()
	cstAlt, err := createConsensusSetTester(name + " - 2")
	if err != nil {
		panic(err)
	}
	defer cstAlt.closeCst()
	cstBackup, err := createConsensusSetTester(name + " - 3")
	if err != nil {
		panic(err)
	}
	defer cstBackup.closeCst()

	return &reorgSets{
		cstMain:   cstMain,
		cstAlt:    cstAlt,
		cstBackup: cstBackup,
	}
}

// save takes all of the blocks in cstMain and moves them to cstBackup.
func (rs *reorgSets) save() {
	for i := types.BlockHeight(1); i <= rs.cstMain.cs.dbBlockHeight(); i++ {
		id, err := rs.cstMain.cs.dbGetPath(i)
		if err != nil {
			panic(err)
		}
		pb, err := rs.cstMain.cs.dbGetBlockMap(id)
		if err != nil {
			panic(err)
		}

		// err is not checked - block may already be in cstBackup.
		_ = rs.cstBackup.cs.AcceptBlock(pb.Block)
	}

	// Check that cstMain and cstBackup are even.
	if rs.cstMain.cs.dbCurrentProcessedBlock().Block.ID() != rs.cstBackup.cs.dbCurrentProcessedBlock().Block.ID() {
		panic("could not save cstMain into cstBackup")
	}
	if rs.cstMain.cs.dbConsensusChecksum() != rs.cstBackup.cs.dbConsensusChecksum() {
		panic("reorg checksums do not match after saving")
	}
}

// extend adds blocks to cstAlt until cstAlt has more weight than cstMain. Then
// cstMain is caught up, causing cstMain to perform a reorg that extends all
// the way to the genesis block.
func (rs *reorgSets) extend() {
	for rs.cstMain.cs.dbBlockHeight() >= rs.cstAlt.cs.dbBlockHeight() {
		_, err := rs.cstAlt.miner.AddBlock()
		if err != nil {
			panic(err)
		}
	}
	for i := types.BlockHeight(1); i <= rs.cstAlt.cs.dbBlockHeight(); i++ {
		id, err := rs.cstAlt.cs.dbGetPath(i)
		if err != nil {
			panic(err)
		}
		pb, err := rs.cstAlt.cs.dbGetBlockMap(id)
		if err != nil {
			panic(err)
		}
		_ = rs.cstMain.cs.AcceptBlock(pb.Block)
	}

	// Check that cstMain and cstAlt are even.
	if rs.cstMain.cs.dbCurrentProcessedBlock().Block.ID() != rs.cstAlt.cs.dbCurrentProcessedBlock().Block.ID() {
		panic("could not save cstMain into cstAlt")
	}
	if rs.cstMain.cs.dbConsensusChecksum() != rs.cstAlt.cs.dbConsensusChecksum() {
		panic("reorg checksums do not match after extending")
	}
}

// restore extends cstBackup until it is ahead of cstMain, and then adds all of
// the blocks from cstBackup to cstMain, causing cstMain to reorg to the state
// of cstBackup.
func (rs *reorgSets) restore() {
	for rs.cstMain.cs.dbBlockHeight() >= rs.cstBackup.cs.dbBlockHeight() {
		_, err := rs.cstBackup.miner.AddBlock()
		if err != nil {
			panic(err)
		}
	}
	for i := types.BlockHeight(1); i <= rs.cstBackup.cs.dbBlockHeight(); i++ {
		id, err := rs.cstBackup.cs.dbGetPath(i)
		if err != nil {
			panic(err)
		}
		pb, err := rs.cstBackup.cs.dbGetBlockMap(id)
		if err != nil {
			panic(err)
		}
		_ = rs.cstMain.cs.AcceptBlock(pb.Block)
	}

	// Check that cstMain and cstBackup are even.
	if rs.cstMain.cs.dbCurrentProcessedBlock().Block.ID() != rs.cstBackup.cs.dbCurrentProcessedBlock().Block.ID() {
		panic("could not save cstMain into cstBackup")
	}
	if rs.cstMain.cs.dbConsensusChecksum() != rs.cstBackup.cs.dbConsensusChecksum() {
		panic("reorg checksums do not match after restoring")
	}
}

// fullReorg saves all of the blocks from cstMain into cstBackup, then extends
// cstAlt until cstMain joins cstAlt in structure. Then cstBackup is extended
// and cstMain is reorg'd back to have all of the original blocks.
func (rs *reorgSets) fullReorg() {
	rs.save()
	rs.extend()
	rs.restore()
}

// TestIntegrationSimpleReorg tries to reorganize a simple block out of, and
// then back into, the consensus set.
func TestIntegrationSimpleReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rs := createReorgSets("TestIntegrationSimpleReorg")

	// Give a simple block to cstMain.
	rs.cstMain.testSimpleBlock()

	// Try to trigger consensus inconsistencies by doing a full reorg on the
	// simple block.
	rs.fullReorg()
}

// TestIntegrationSiacoinReorg tries to reorganize a siacoin output block out
// of, and then back into, the consensus set.
func TestIntegrationSiacoinReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rs := createReorgSets("TestIntegrationSiacoinReorg")

	// Give a simple block to cstMain.
	rs.cstMain.testSimpleBlock()

	// Try to trigger consensus inconsistencies by doing a full reorg on the
	// simple block.
	rs.fullReorg()
}

/// BREAK ///
/// BREAK ///
/// BREAK ///

// complexBlockSet puts a set of blocks with many types of transactions into
// the consensus set.
func (cst *consensusSetTester) complexBlockSet() error {
	cst.testSimpleBlock()
	cst.testSpendSiacoinsBlock()

	// COMPATv0.4.0
	//
	// Mine enough blocks to get above the file contract hardfork threshold
	// (10).
	for i := 0; i < 10; i++ {
		block, _ := cst.miner.FindBlock()
		err := cst.cs.AcceptBlock(block)
		if err != nil {
			return err
		}
	}

	err := cst.testFileContractsBlocks()
	if err != nil {
		return err
	}
	err = cst.testSpendSiafundsBlock()
	if err != nil {
		return err
	}
	return nil
}

// TestComplexForking adds every type of test block into two parallel chains of
// consensus, and then forks to a new chain, forcing the whole structure to be
// reverted.
func TestComplexForking(t *testing.T) {
	/*
		if testing.Short() {
			t.SkipNow()
		}
		cstMain, err := createConsensusSetTester("TestComplexForking - 1")
		if err != nil {
			t.Fatal(err)
		}
		defer cstMain.closeCst()
		cstAlt, err := createConsensusSetTester("TestComplexForking - 2")
		if err != nil {
			t.Fatal(err)
		}
		defer cstAlt.closeCst()
		cstBackup, err := createConsensusSetTester("TestComplexForking - 3")
		if err != nil {
			t.Fatal(err)
		}
		defer cstBackup.closeCst()

		// Give each type of major block to cstMain.
		err = cstMain.complexBlockSet()
		if err != nil {
			t.Error(err)
		}

		// Give all the blocks in cstMain to cstBackup - as a holding place.
		var cstMainBlocks []types.Block
		pb := cstMain.cs.currentProcessedBlock()
		for pb.Block.ID() != cstMain.cs.blockRoot.Block.ID() {
			cstMainBlocks = append([]types.Block{pb.Block}, cstMainBlocks...) // prepend
			pb = cstMain.cs.db.getBlockMap(pb.Block.ParentID)
		}

		for _, block := range cstMainBlocks {
			// Some blocks will return errors.
			_ = cstBackup.cs.AcceptBlock(block)
		}
		if cstBackup.cs.currentBlockID() != cstMain.cs.currentBlockID() {
			t.Error("cstMain and cstBackup do not share the same path")
		}
		if cstBackup.cs.consensusSetHash() != cstMain.cs.consensusSetHash() {
			t.Error("cstMain and cstBackup do not share a consensus set hash")
		}

		// Mine 3 blocks on cstAlt, then all the block types, to give it a heavier
		// weight, then give all of its blocks to cstMain. This will cause a complex
		// fork to happen.
		for i := 0; i < 3; i++ {
			block, _ := cstAlt.miner.FindBlock()
			err = cstAlt.cs.AcceptBlock(block)
			if err != nil {
				t.Fatal(err)
			}
		}
		err = cstAlt.complexBlockSet()
		if err != nil {
			t.Error(err)
		}
		var cstAltBlocks []types.Block
		pb = cstAlt.cs.currentProcessedBlock()
		for pb.Block.ID() != cstAlt.cs.blockRoot.Block.ID() {
			cstAltBlocks = append([]types.Block{pb.Block}, cstAltBlocks...) // prepend
			pb = cstAlt.cs.db.getBlockMap(pb.Block.ParentID)
		}
		fmt.Println(cstMain.cs.dbBlockHeight())
		for i, block := range cstAltBlocks {
			// Some blocks will return errors.
			fmt.Println(i, cstMain.cs.dbBlockHeight())
			_ = cstMain.cs.AcceptBlock(block)
		}
		if cstMain.cs.currentBlockID() != cstAlt.cs.currentBlockID() {
			t.Error("cstMain and cstAlt do not share the same path")
		}
		if cstMain.cs.consensusSetHash() != cstAlt.cs.consensusSetHash() {
			t.Error("cstMain and cstAlt do not share the same consensus set hash")
		}

		// Mine 6 blocks on cstBackup and then give those blocks to cstMain, which will
		// cause cstMain to switch back to its old chain. cstMain will then have created,
		// reverted, and reapplied all the significant types of blocks.
		for i := 0; i < 6; i++ {
			block, _ := cstBackup.miner.FindBlock()
			err = cstBackup.cs.AcceptBlock(block)
			if err != nil {
				t.Fatal(err)
			}
		}
		var cstBackupBlocks []types.Block
		pb = cstBackup.cs.currentProcessedBlock()
		for pb.Block.ID() != cstBackup.cs.blockRoot.Block.ID() {
			cstBackupBlocks = append([]types.Block{pb.Block}, cstBackupBlocks...) // prepend
			pb = cstBackup.cs.db.getBlockMap(pb.Block.ParentID)
		}
		for _, block := range cstBackupBlocks {
			// Some blocks will return errors.
			_ = cstMain.cs.AcceptBlock(block)
		}
		if cstMain.cs.currentBlockID() != cstBackup.cs.currentBlockID() {
			t.Error("cstMain and cstBackup do not share the same path")
		}
		if cstMain.cs.consensusSetHash() != cstBackup.cs.consensusSetHash() {
			t.Error("cstMain and cstBackup do not share the same consensus set hash")
		}
	*/
}
