package main

import (
	"github.com/ethereum/go-ethereum/crypto"
	"path/filepath"
	"testing"
)

func TestGenerate(t *testing.T) {
	outdir := t.TempDir()
	cfg := generatorConfig{
		txInterval:   1,
		txCount:      10,
		forkInterval: 2,
		chainLength:  30,
		outputDir:    outdir,
		outputs:      outputFunctionNames(),
	}
	cfg, err := cfg.withDefaults()
	if err != nil {
		t.Fatal(err)
	}
	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	g := newGenerator(cfg, crypto.PubkeyToAddress(pk.PublicKey))
	if err := g.run(pk); err != nil {
		t.Fatal(err)
	}

	names, _ := filepath.Glob(filepath.Join(outdir, "*"))
	t.Log("output files:", names)
}
