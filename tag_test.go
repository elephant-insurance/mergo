package mergo

import (
	"testing"

	"github.com/davecgh/go-spew/spew"

	dig "github.com/elephant-insurance/diagnostics/v2"
	log "github.com/elephant-insurance/logging/v2"
)

type testTagCfg struct {
	UnfinalString string
	FinalString   string `config:"final"`
	UnfinalStrPtr *string
	FinalStrPtr   *string `config:"final"`
}

var (
	bus  = `base_unfinal_string`
	ous  = `override_unfinal_string`
	bfs  = `base_final_string`
	ofs  = `override_final_string`
	busp = `base_unfinal_string_ptr`
	ousp = `override_unfinal_string_ptr`
	bfsp = `base_final_string_ptr`
	ofsp = `override_final_string_ptr`
)

func TestFinalTag(t *testing.T) {
	basecfg := testTagCfg{bus, bfs, &busp, &bfsp}
	ovrcfg := testTagCfg{ous, ofs, &ousp, &ofsp}
	err := Merge(&basecfg, &ovrcfg, WithOverride)
	if err != nil {
		t.Fatal(`error running Merge: ` + err.Error())
	}

	// test that the merge worked as expected:
	if basecfg.UnfinalString != ous || basecfg.UnfinalStrPtr == nil || *basecfg.UnfinalStrPtr != ousp {
		spew.Dump(basecfg, ovrcfg)
		t.Fatal(`failed to overwrite non-final fields properly`)
	}
	if basecfg.FinalString != bfs || basecfg.FinalStrPtr == nil || *basecfg.FinalStrPtr != bfsp {
		spew.Dump(basecfg, ovrcfg)
		t.Fatal(`overwrote final fields`)
	}
}

func TestMergeConfig(t *testing.T) {
	rc := RequiredConfig{Environment: "test"}
	rc2 := RequiredConfig{LogLevel: "info"}
	ds := (&dig.Settings{}).ForTesting()
	ls := (&log.Settings{}).ForTesting()
	basecfg := MSGDConfig{rc, *ds, *ls}
	ovrcfg := MSGDConfig{rc2, *ds, *ls}

	err := Merge(&basecfg, &ovrcfg, WithOverride)
	if err != nil {
		t.Fatal(`error running Merge: ` + err.Error())
	}
}

// This is just a local replica of configuration.RequiredConfig for testing
type RequiredConfig struct {
	OverrideConfigPath string `config:"final,optional" yaml:"OverrideConfigPath"`
	Environment        string `yaml:"Environment"`
	LogLevel           string `yaml:"LogLevel"`
	Version            string `yaml:"Version"`
	Branch             string `yaml:"Branch"`
	Commit             string `yaml:"Commit"`
	ImageTag           string `yaml:"ImageTag"`
	Build              string `yaml:"Build"`
	DateBuilt          string `yaml:"DateBuilt"`
}

// local replica of geo-data config
type MSGDConfig struct {
	RequiredConfig `yaml:"RequiredConfig"`
	Diagnostics    dig.Settings `yaml:"Diagnostics"`
	Logging        log.Settings `yaml:"Logging"`
}
