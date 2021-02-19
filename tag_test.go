package mergo

import (
	"testing"

	"github.com/davecgh/go-spew/spew"

	cfg "github.com/elephant-insurance/configuration"
	dig "github.com/elephant-insurance/diagnostics/v2"
	log "github.com/elephant-insurance/logging/v2"
	msgdcfg "github.com/elephant-insurance/ms-geo-data/config"
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
	rc := cfg.RequiredConfig{Environment: "test"}
	rc2 := cfg.RequiredConfig{LogLevel: "info"}
	ds := (&dig.Settings{}).ForTesting()
	ls := (&log.Settings{}).ForTesting()
	basecfg := msgdcfg.MSGDConfig{rc, *ds, *ls}
	ovrcfg := msgdcfg.MSGDConfig{rc2, *ds, *ls}

	err := Merge(&basecfg, &ovrcfg, WithOverride)
	if err != nil {
		t.Fatal(`error running Merge: ` + err.Error())
	}
}
