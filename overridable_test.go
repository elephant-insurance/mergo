package mergo

import (
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestOverrideWithEnvironment(t *testing.T) {
	otsc1 := OvrTestSubConfig{&note1, value1}
	otsc2 := OvrTestSubConfig{&note2, value2}
	otc1 := OvrTestConfig{name1, &desc1, otsc1}
	otc2 := OvrTestConfig{name2, &desc2, otsc2}

	err := MergeWithOverwrite(&otc1, &otc2)
	if err != nil {
		t.Fatal(`error merging override configs: ` + err.Error())
	}
	if otc1.OvrTestSubConfig.Value != otc2.OvrTestSubConfig.Value {
		spew.Dump(otc1, otc2)
		t.Fatal(`merge did not produce expected result`)
	}
	if otc1.OvrTestSubConfig.Note == nil || otc2.OvrTestSubConfig.Note == nil || *otc1.OvrTestSubConfig.Note != *otc2.OvrTestSubConfig.Note {
		spew.Dump(otc1, otc2)
		t.Fatal(`merge did not produce expected result for string pointer`)
	}

	os.Setenv(`OVRTSC_Value`, `goober!`)
	os.Setenv(`OVRTSC_Note`, `little note`)
	err = MergeWithOverwrite(&otc1, &otc2)
	if err != nil {
		t.Fatal(`error merging override configs with environment: ` + err.Error())
	}
	if otc1.OvrTestSubConfig.Value != `goober!` {
		spew.Dump(otc1, otc2)
		t.Fatal(`merge did not produce expected result with environment`)
	}
	if otc1.OvrTestSubConfig.Note == nil || *otc1.OvrTestSubConfig.Note != `little note` {
		spew.Dump(otc1, otc2)
		t.Fatal(`merge did not produce expected result for string pointer with environment`)
	}
}

type OvrTestConfig struct {
	Name        string
	Description *string
	OvrTestSubConfig
}

type OvrTestSubConfig struct {
	Note  *string
	Value string
}

func (otc OvrTestConfig) GetEnvironmentSetting(fieldName string) string {
	return `OTC_` + fieldName
}

func (otsc OvrTestSubConfig) GetEnvironmentSetting(fieldName string) string {
	return `OVRTSC_` + fieldName
}

var (
	name1  string = `name1`
	name2  string = `name2`
	value1 string = `value1`
	value2 string = `value2`
	desc1  string = `desc1`
	desc2  string = `desc2`
	note1  string = `note1`
	note2  string = `note2`
	count1 int    = 1
	count2 int    = 2
	index1 int    = 10
	index2 int    = 20
)
