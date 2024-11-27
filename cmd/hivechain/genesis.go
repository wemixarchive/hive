package main

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/common/math"
	qbftengine "github.com/ethereum/go-ethereum/consensus/qbft/engine"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"golang.org/x/exp/slices"
)

var initialBalance, _ = new(big.Int).SetString("1000000000000000000000000000000000000", 10)

const (
	genesisBaseFee = params.InitialBaseFee
	blocktimeSec   = 10 // hard-coded in core.GenerateChain
	cliqueEpoch    = 30000
)

var (
	cliqueSignerKey  = knownAccounts[8].key
	cliqueSignerAddr = crypto.PubkeyToAddress(cliqueSignerKey.PublicKey)
)

// Ethereum mainnet forks in order of introduction.
var (
	allForkNames = append(numberBasedForkNames, timeBasedForkNames...)
	lastFork     = allForkNames[len(allForkNames)-1]

	numberBasedForkNames = []string{
		"homestead",
		"tangerinewhistle",
		"spuriousdragon",
		"byzantium",
		"constantinople",
		"petersburg",
		"istanbul",
		"muirglacier",
		"berlin",
		"london",
		"arrowglacier",
		"grayglacier",
		"merge",
	}

	timeBasedForkNames = []string{
		"shanghai",
		"cancun",
		"prague",
	}
)

// createChainConfig creates a chain configuration.
func (cfg *generatorConfig) createChainConfig(val common.Address) *params.ChainConfig {
	chaincfg := new(params.ChainConfig)

	chainid, _ := new(big.Int).SetString("3503995874084926", 10)
	chaincfg.ChainID = chainid

	// Set consensus algorithm.
	if cfg.clique {
		chaincfg.Clique = &params.CliqueConfig{
			Period: blocktimeSec,
			Epoch:  cliqueEpoch,
		}
	} else {
		chaincfg.Ethash = new(params.EthashConfig)
	}

	chaincfg.HomesteadBlock = new(big.Int)
	//chaincfg.DAOForkBlock = new(big.Int)  QBFT engine cannot handle DAOFork block because of Extra field
	//chaincfg.DAOForkSupport = true
	chaincfg.EIP150Block = new(big.Int)
	chaincfg.EIP155Block = new(big.Int)
	chaincfg.EIP158Block = new(big.Int)
	chaincfg.ByzantiumBlock = new(big.Int)
	chaincfg.ConstantinopleBlock = new(big.Int)
	chaincfg.PetersburgBlock = new(big.Int)
	chaincfg.IstanbulBlock = new(big.Int)
	chaincfg.MuirGlacierBlock = new(big.Int)
	chaincfg.BerlinBlock = new(big.Int)
	chaincfg.LondonBlock = new(big.Int)
	chaincfg.ArrowGlacierBlock = new(big.Int)
	chaincfg.GrayGlacierBlock = new(big.Int)
	chaincfg.PangyoBlock = new(big.Int)
	chaincfg.ApplepieBlock = new(big.Int)
	chaincfg.BriocheBlock = new(big.Int)
	chaincfg.MontBlancBlock = new(big.Int)
	chaincfg.Brioche = &params.BriocheConfig{
		BlockReward:       big.NewInt(1e18),
		FirstHalvingBlock: big.NewInt(50),
		HalvingPeriod:     big.NewInt(50),
		FinishRewardBlock: big.NewInt(450),
		HalvingTimes:      8,
		HalvingRate:       50,
	}

	ebps := uint64(6)
	br, _ := new(big.Int).SetString("1000000000000000000", 10)
	bm := "validator"
	vsm := "blockheader"
	mb := common.HexToAddress("0x0000000000000000000000000000000000000000")
	vals := make([]common.Address, 1)
	vals[0] = val
	mrts := uint64(4)
	chaincfg.QBFT = &params.QBFTConfig{
		EpochLength:              10,
		BlockPeriodSeconds:       3,
		EmptyBlockPeriodSeconds:  &ebps,
		RequestTimeoutSeconds:    1000,
		ProposerPolicy:           0,
		BlockReward:              (*math.HexOrDecimal256)(br),
		BeneficiaryMode:          &bm,
		MiningBeneficiary:        &mb,
		ValidatorSelectionMode:   &vsm,
		Validators:               vals,
		MaxRequestTimeoutSeconds: &mrts,
	}

	/*
		// Apply forks.
		forks := cfg.forkBlocks()
		for fork, b := range forks {
			timestamp := cfg.blockTimestamp(b)

			switch fork {
			// number-based forks
			case "homestead":
				chaincfg.HomesteadBlock = new(big.Int).SetUint64(b)
			case "tangerinewhistle":
				chaincfg.EIP150Block = new(big.Int).SetUint64(b)
			case "spuriousdragon":
				chaincfg.EIP155Block = new(big.Int).SetUint64(b)
				chaincfg.EIP158Block = new(big.Int).SetUint64(b)
			case "byzantium":
				chaincfg.ByzantiumBlock = new(big.Int).SetUint64(b)
			case "constantinople":
				chaincfg.ConstantinopleBlock = new(big.Int).SetUint64(b)
			case "petersburg":
				chaincfg.PetersburgBlock = new(big.Int).SetUint64(b)
			case "istanbul":
				chaincfg.IstanbulBlock = new(big.Int).SetUint64(b)
			case "muirglacier":
				chaincfg.MuirGlacierBlock = new(big.Int).SetUint64(b)
			case "berlin":
				chaincfg.BerlinBlock = new(big.Int).SetUint64(b)
			case "london":
				chaincfg.LondonBlock = new(big.Int).SetUint64(b)
			case "arrowglacier":
				chaincfg.ArrowGlacierBlock = new(big.Int).SetUint64(b)
			case "grayglacier":
				chaincfg.GrayGlacierBlock = new(big.Int).SetUint64(b)
			case "merge":
				chaincfg.MergeNetsplitBlock = new(big.Int).SetUint64(b)
				chaincfg.TerminalTotalDifficultyPassed = true
			// time-based forks
			case "shanghai":
				chaincfg.ShanghaiTime = &timestamp
			case "cancun":
				chaincfg.CancunTime = &timestamp
			case "prague":
				chaincfg.PragueTime = &timestamp
			default:
				panic(fmt.Sprintf("unknown fork name %q", fork))
			}
		}
		// Special case for merged-from-genesis networks.
		// Need to assign TTD here because the genesis block won't be processed by GenerateChain.
		if chaincfg.MergeNetsplitBlock != nil && chaincfg.MergeNetsplitBlock.Sign() == 0 {
			chaincfg.TerminalTotalDifficulty = cfg.genesisDifficulty()
		}
		//ttd := new(big.Int).Set(params.GenesisDifficulty)
		// We intend last block to be `ForkchoiceUpdated`
		//chaincfg.TerminalTotalDifficulty = ttd.Add(ttd, big.NewInt(500))
	*/

	return chaincfg
}

func (cfg *generatorConfig) genesisDifficulty() *big.Int {
	if cfg.clique {
		return big.NewInt(1)
	}
	//return new(big.Int).Set(params.MinimumDifficulty)
	return big.NewInt(1)
}

// createGenesis creates the genesis block and config.
func (cfg *generatorConfig) createGenesis(val common.Address) *core.Genesis {
	var g core.Genesis
	g.Config = cfg.createChainConfig(val)

	// Block attributes.
	g.Difficulty = cfg.genesisDifficulty()
	if cfg.clique {
		g.ExtraData = cliqueInit(cliqueSignerKey)
	} else {
		vals := make([]common.Address, 1)
		vals[0] = val
		header := g.ToBlock().Header()
		qbftengine.ApplyHeaderQBFTExtra(
			header,
			func(qbftExtra *types.QBFTExtra) error {
				qbftExtra.Validators = vals
				return nil
			})
		g.ExtraData = header.Extra
	}
	g.GasLimit = params.GenesisGasLimit * 8
	zero := new(big.Int)
	if g.Config.IsLondon(zero) {
		g.BaseFee = big.NewInt(genesisBaseFee)
	}

	// Initialize allocation.
	// Here we add balance to known accounts and initialize built-in contracts.
	g.Alloc = make(core.GenesisAlloc)
	g.Alloc[val] = core.GenesisAccount{Balance: initialBalance} // validator account must have initial balance
	for _, acc := range knownAccounts {
		g.Alloc[acc.addr] = core.GenesisAccount{Balance: initialBalance}
	}
	add4788Contract(g.Alloc)
	addSnapTestContract(g.Alloc)
	addEmitContract(g.Alloc)

	return &g
}

func add4788Contract(ga core.GenesisAlloc) {
	ga[params.BeaconRootsStorageAddress] = core.GenesisAccount{
		Balance: big.NewInt(42),
		Code:    common.FromHex("0x3373fffffffffffffffffffffffffffffffffffffffe14604d57602036146024575f5ffd5b5f35801560495762001fff810690815414603c575f5ffd5b62001fff01545f5260205ff35b5f5ffd5b62001fff42064281555f359062001fff015500"),
	}
}

func addSnapTestContract(ga core.GenesisAlloc) {
	addr := common.HexToAddress("0x8bebc8ba651aee624937e7d897853ac30c95a067")
	h := common.HexToHash
	ga[addr] = core.GenesisAccount{
		Balance: big.NewInt(1),
		Nonce:   1,
		Storage: map[common.Hash]common.Hash{
			h("0x01"): h("0x01"),
			h("0x02"): h("0x02"),
			h("0x03"): h("0x03"),
		},
	}
}

const emitAddr = "0x7dcd17433742f4c0ca53122ab541d0ba67fc27df"

func addEmitContract(ga core.GenesisAlloc) {
	addr := common.HexToAddress(emitAddr)
	ga[addr] = core.GenesisAccount{
		Balance: new(big.Int),
		Code:    emitCode,
	}
}

// forkBlocks computes the block numbers where forks occur. Forks get enabled based on the
// forkInterval. If the total number of requested blocks (chainLength) is lower than
// necessary, the remaining forks activate on the last chain block.
func (cfg *generatorConfig) forkBlocks() map[string]uint64 {
	lastIndex := cfg.lastForkIndex()
	forks := allForkNames[:lastIndex+1]
	forkBlocks := make(map[string]uint64)
	for block := 0; block <= cfg.chainLength && len(forks) > 0; {
		fork := forks[0]
		forks = forks[1:]
		forkBlocks[fork] = uint64(block)
		block += cfg.forkInterval
	}
	for _, f := range forks {
		forkBlocks[f] = uint64(cfg.chainLength)
	}
	return forkBlocks
}

// lastForkIndex returns the index of the latest enabled for in allForkNames.
func (cfg *generatorConfig) lastForkIndex() int {
	if cfg.lastFork == "" || cfg.lastFork == "frontier" {
		return len(allForkNames) - 1
	}
	index := slices.Index(allForkNames, strings.ToLower(cfg.lastFork))
	if index == -1 {
		panic(fmt.Sprintf("unknown lastFork name %q", cfg.lastFork))
	}
	return index
}

func (cfg *generatorConfig) blockTimestamp(num uint64) uint64 {
	return num * blocktimeSec
}

// cliqueInit creates the genesis extradata for a clique network with one signer.
func cliqueInit(signer *ecdsa.PrivateKey) []byte {
	vanity := make([]byte, 32)
	copy(vanity, "hivechain")
	d := append(vanity, cliqueSignerAddr[:]...)
	d = append(d, make([]byte, 65)...) // signature
	return d
}
