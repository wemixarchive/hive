package main

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/ethereum/go-ethereum/node"
	"math"
	"math/big"
	"math/rand"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/clique"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/triedb"
	"golang.org/x/exp/slices"
)

// generatorConfig is the configuration of the chain generator.
type generatorConfig struct {
	// genesis options
	forkInterval int    // number of blocks between forks
	lastFork     string // last enabled fork
	clique       bool   // create a clique chain

	// chain options
	txInterval  int // frequency of blocks containing transactions
	txCount     int // number of txs in block
	chainLength int // number of generated blocks

	// output options
	outputs   []string // enabled outputs
	outputDir string   // path where output files should be placed
}

func (cfg generatorConfig) withDefaults() (generatorConfig, error) {
	if cfg.lastFork == "" {
		cfg.lastFork = lastFork
	}
	if cfg.txInterval == 0 {
		cfg.txInterval = 1
	}
	if cfg.outputs == nil {
		cfg.outputs = []string{"genesis", "chain", "txinfo"}
	}
	return cfg, nil
}

// generator is the central object in the chain generation process.
// It holds the configuration, state, and all instantiated transaction generators.
type generator struct {
	cfg      generatorConfig
	genesis  *core.Genesis
	td       *big.Int
	accounts []genAccount
	rand     *rand.Rand

	// Modifier lists.
	virgins   []*modifierInstance
	mods      []*modifierInstance
	modOffset int

	// for write/export
	blockchain *core.BlockChain
}

type modifierInstance struct {
	name string
	blockModifier
}

type genAccount struct {
	addr common.Address
	key  *ecdsa.PrivateKey
}

func newGenerator(cfg generatorConfig, val common.Address) *generator {
	genesis := cfg.createGenesis(val)
	return &generator{
		cfg:      cfg,
		genesis:  genesis,
		rand:     rand.New(rand.NewSource(10)),
		td:       new(big.Int).Set(genesis.Difficulty),
		virgins:  cfg.createBlockModifiers(),
		accounts: slices.Clone(knownAccounts),
	}
}

func (cfg *generatorConfig) createBlockModifiers() (list []*modifierInstance) {
	for name, new := range modRegistry {
		list = append(list, &modifierInstance{
			name:          name,
			blockModifier: new(),
		})
	}
	slices.SortFunc(list, func(a, b *modifierInstance) int {
		return strings.Compare(a.name, b.name)
	})
	return list
}

// run produces a chain and writes it.
func (g *generator) run(pk *ecdsa.PrivateKey) error {
	db := rawdb.NewMemoryDatabase()
	engine := g.createConsensusEngine(db, pk)

	// Init genesis block.
	trieconfig := *triedb.HashDefaults
	trieconfig.Preimages = true
	triedb := triedb.NewDatabase(db, &trieconfig)
	genesis := g.genesis.MustCommit(db, triedb)

	// Create the blocks.
	chain, _ := core.GenerateChain(g.genesis.Config, genesis, engine, db, g.cfg.chainLength, g.modifyBlock)

	// Import the chain. This runs all block validation rules.
	bc, err := g.importChain(engine, chain)
	if err != nil {
		return err
	}

	g.blockchain = bc
	return g.write()
}

func (g *generator) createConsensusEngine(db ethdb.Database, pk *ecdsa.PrivateKey) consensus.Engine {
	var inner consensus.Engine
	if g.genesis.Config.Clique != nil {
		cliqueEngine := clique.New(g.genesis.Config.Clique, db)
		cliqueEngine.Authorize(cliqueSignerAddr, func(signer accounts.Account, mimeType string, message []byte) ([]byte, error) {
			sig, err := crypto.Sign(crypto.Keccak256(message), cliqueSignerKey)
			return sig, err
		})
		inner = instaSeal{cliqueEngine}
		return beacon.New(inner)
	}
	/*
		opts, err := bind.NewKeyedTransactorWithChainID(pk, params.AllEthashProtocolChanges.ChainID)
		if err != nil {
			return nil
		}
		g.genesis.Alloc = types.GenesisAlloc{
			opts.From: {Balance: new(big.Int).Sub(new(big.Int).Lsh(common.Big1, 128), common.Big1)},
		}*/
	backend := simulated.NewWbftBackend(types.GenesisAlloc{}, func(nodeConf *node.Config, ethConf *ethconfig.Config) {
		ethConf.Genesis = g.genesis
		nodeConf.P2P.PrivateKey = pk
	})
	return backend.Engine()
}

func (g *generator) importChain(engine consensus.Engine, chain []*types.Block) (*core.BlockChain, error) {
	db := rawdb.NewMemoryDatabase()
	cacheconfig := core.DefaultCacheConfigWithScheme("hash")
	cacheconfig.Preimages = true
	vmconfig := vm.Config{EnablePreimageRecording: true}
	blockchain, err := core.NewBlockChain(db, cacheconfig, g.genesis, nil, engine, vmconfig, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("can't create blockchain: %v", err)
	}

	i, err := blockchain.InsertChain(chain)
	if err != nil {
		blockchain.Stop()
		return nil, fmt.Errorf("chain validation error (block %d): %v", chain[i].Number(), err)
	}
	return blockchain, nil
}

func (g *generator) modifyBlock(i int, gen *core.BlockGen) {
	fmt.Println("generating block", gen.Number())
	if g.genesis.Config.Clique != nil {
		g.setClique(i, gen)
	}
	g.setDifficulty(i, gen)
	g.setParentBeaconRoot(i, gen)
	g.runModifiers(i, gen)
}

func (g *generator) setClique(i int, gen *core.BlockGen) {
	mergeblock := g.genesis.Config.MergeNetsplitBlock
	if mergeblock != nil && gen.Number().Cmp(mergeblock) >= 0 {
		return
	}

	gen.SetCoinbase(cliqueSignerAddr)
	// Add a positive vote to keep the signer in the set.
	gen.SetNonce(types.BlockNonce{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	// The clique engine requires the block to have blank extra-data of the correct length before sealing.
	gen.SetExtra(make([]byte, 32+65))
}

func (g *generator) setDifficulty(i int, gen *core.BlockGen) {
	chaincfg := g.genesis.Config
	mergeblock := chaincfg.MergeNetsplitBlock
	if mergeblock == nil {
		mergeblock = new(big.Int).SetUint64(math.MaxUint64)
	}
	switch gen.Number().Cmp(mergeblock) {
	case 1:
		gen.SetPoS()
	case 0:
		gen.SetPoS()
		chaincfg.TerminalTotalDifficulty = new(big.Int).Set(g.td)
	default:
		g.td = g.td.Add(g.td, gen.Difficulty())
	}
}

func (g *generator) setParentBeaconRoot(i int, gen *core.BlockGen) {
	if g.genesis.Config.IsCancun(gen.Number(), gen.Timestamp()) {
		var h common.Hash
		g.rand.Read(h[:])
		gen.SetParentBeaconRoot(h)
	}
}

// runModifiers executes the chain modifiers.
func (g *generator) runModifiers(i int, gen *core.BlockGen) {
	totalMods := len(g.mods) + len(g.virgins)
	if totalMods == 0 || g.cfg.txInterval == 0 || i%g.cfg.txInterval != 0 {
		return
	}

	ctx := &genBlockContext{index: i, block: gen, gen: g}

	// Modifier scheduling: we cycle through the available modifiers until enough have
	// executed successfully. It also stops when all of them return false from apply()
	// because this usually means there is no gas left.
	count := 0
	refused := 0 // count of consecutive times apply() returned false
	run := func(mod *modifierInstance) bool {
		ok := mod.apply(ctx)
		if ok {
			fmt.Println("    -", mod.name)
			count++
			refused = 0
		} else {
			refused++
		}
		return ok
	}

	// In order to avoid a pathological situation where a modifier never executes because
	// of unfortunate scheduling, we first try modifiers from g.virgins.
	for i := 0; i < len(g.virgins) && count < g.cfg.txCount; i++ {
		mod := g.virgins[i]
		if run(mod) {
			g.mods = append(g.mods, mod)
			g.virgins = append(g.virgins[:i], g.virgins[i+1:]...)
			i--
		}
	}
	// If there is any space left, fill it using g.mods.
	for len(g.mods) > 0 && count < g.cfg.txCount && refused < totalMods {
		index := g.modOffset % len(g.mods)
		run(g.mods[index])
		g.modOffset++
	}
}

// instaSeal wraps a consensus engine with instant block sealing. When a block is produced
// using FinalizeAndAssemble, it also applies Seal.
type instaSeal struct{ consensus.Engine }

// FinalizeAndAssemble implements consensus.Engine, accumulating the block and uncle rewards,
// setting the final state and assembling the block.
func (e instaSeal) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt, withdrawals []*types.Withdrawal) (*types.Block, error) {
	block, err := e.Engine.FinalizeAndAssemble(chain, header, state, txs, uncles, receipts, withdrawals)
	if err != nil {
		return nil, err
	}
	sealedBlock := make(chan *types.Block, 1)
	if err = e.Engine.Seal(chain, block, sealedBlock, nil); err != nil {
		return nil, err
	}
	return <-sealedBlock, nil
}
