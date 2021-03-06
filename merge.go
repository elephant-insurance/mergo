// Copyright 2013 Dario Castañé. All rights reserved.
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Based on src/pkg/reflect/deepequal.go from official
// golang's stdlib.

package mergo

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

const DefaultEnvironmentSettingPrefix string = `MSVC_`

var StructFieldDict = map[reflect.Type]map[string]FieldInfo{}

func hasMergeableFields(dst reflect.Value) (exported bool) {
	for i, n := 0, dst.NumField(); i < n; i++ {
		field := dst.Type().Field(i)
		if field.Anonymous && dst.Field(i).Kind() == reflect.Struct {
			exported = exported || hasMergeableFields(dst.Field(i))
		} else if isExportedComponent(&field) {
			exported = exported || len(field.PkgPath) == 0
		}
	}
	return
}

func isExportedComponent(field *reflect.StructField) bool {
	pkgPath := field.PkgPath
	if len(pkgPath) > 0 {
		return false
	}
	c := field.Name[0]
	if 'a' <= c && c <= 'z' || c == '_' {
		return false
	}
	return true
}

type Config struct {
	Overwrite                    bool
	AppendSlice                  bool
	TypeCheck                    bool
	Transformers                 Transformers
	overwriteWithEmptyValue      bool
	overwriteSliceWithEmptyValue bool
	sliceDeepCopy                bool
	debug                        bool
}

type Transformers interface {
	Transformer(reflect.Type) func(dst, src reflect.Value) error
}

// Traverses recursively both values, assigning src's fields values to dst.
// The map argument tracks comparisons that have already been seen, which allows
// short circuiting on recursive types.
func deepMerge(dst, src reflect.Value, visited map[uintptr]*visit, depth int, config *Config) (err error) {
	overwrite := config.Overwrite
	typeCheck := config.TypeCheck
	overwriteWithEmptySrc := config.overwriteWithEmptyValue
	overwriteSliceWithEmptySrc := config.overwriteSliceWithEmptyValue
	sliceDeepCopy := config.sliceDeepCopy

	if !src.IsValid() {
		return
	}
	if dst.CanAddr() {
		addr := dst.UnsafeAddr()
		h := 17 * addr
		seen := visited[h]
		typ := dst.Type()
		for p := seen; p != nil; p = p.next {
			if p.ptr == addr && p.typ == typ {
				return nil
			}
		}
		// Remember, remember...
		visited[h] = &visit{addr, typ, seen}
	}

	if config.Transformers != nil && !isEmptyValue(dst) {
		if fn := config.Transformers.Transformer(dst.Type()); fn != nil {
			err = fn(dst, src)
			return
		}
	}

	var covr Overridable
	var ok bool

	ovrtype := reflect.TypeOf((*Overridable)(nil)).Elem()
	// spew.Dump("DST: ", dst.IsValid(), "TYPE: ", dst.Type().IsValid())
	overridable := dst.IsValid() && !dst.IsZero() && dst.Type().Implements(ovrtype)

	// spew.Dump(fmt.Sprintf("Type %v implements overridable: %v", dst.Type().Name(), overridable))
	if overridable {
		covr, ok = dst.Interface().(Overridable)
		if !ok {
			overridable = false
			// spew.Dump(fmt.Sprintf("Error casting %v to overridable type", dst.Type().Name()))
		}
	}
	switch dst.Kind() {
	case reflect.Struct:
		baseType := dst.Type()
		if _, ok := StructFieldDict[baseType]; !ok {
			StructFieldDict[baseType] = map[string]FieldInfo{}
		}

		if hasMergeableFields(dst) {
			for i, n := 0, dst.NumField(); i < n; i++ {
				df := dst.Type().Field(i)
				di := dst.Field(i)
				dfi := parseField(df)
				StructFieldDict[baseType][dfi.Name] = dfi

				overridden := false
				if !dfi.Final {
					if overridable && !dfi.Complex {
						// spew.Dump("BEFORE", di.Interface())
						overridden = valueFromEnvironment(di, covr.GetEnvironmentSetting(df.Name))
						// spew.Dump("AFTER", di.Interface())
						// spew.Dump("environment variable for value %v: %v", dst.Type().Name(), covr.GetEnvironmentSetting(df.Name))
					} else if !dfi.Complex {
						// Handle the case where the struct does not have an overridable method but is still overridden with the default prefix
						overridden = valueFromEnvironment(di, DefaultEnvironmentSettingPrefix+df.Name)
					}
					// TODO: PREVENT THIS IF WE GET THE VALUE FROM THE ENVIRONMENT:
					if !overridden {
						if err = deepMerge(dst.Field(i), src.Field(i), visited, depth+1, config); err != nil {
							return
						}
					}
				}
			}
		} else {
			if dst.CanSet() && (isReflectNil(dst) || overwrite) && (!isEmptyValue(src) || overwriteWithEmptySrc) {
				dst.Set(src)
			}
		}
	case reflect.Map:
		if dst.IsNil() && !src.IsNil() {
			if dst.CanSet() {
				dst.Set(reflect.MakeMap(dst.Type()))
			} else {
				dst = src
				return
			}
		}

		if src.Kind() != reflect.Map {
			if overwrite {
				dst.Set(src)
			}
			return
		}

		for _, key := range src.MapKeys() {
			srcElement := src.MapIndex(key)
			if !srcElement.IsValid() {
				continue
			}
			dstElement := dst.MapIndex(key)
			switch srcElement.Kind() {
			case reflect.Chan, reflect.Func, reflect.Map, reflect.Interface, reflect.Slice:
				if srcElement.IsNil() {
					if overwrite {
						dst.SetMapIndex(key, srcElement)
					}
					continue
				}
				fallthrough
			default:
				if !srcElement.CanInterface() {
					continue
				}
				switch reflect.TypeOf(srcElement.Interface()).Kind() {
				case reflect.Struct:
					fallthrough
				case reflect.Ptr:
					fallthrough
				case reflect.Map:
					srcMapElm := srcElement
					dstMapElm := dstElement
					if srcMapElm.CanInterface() {
						srcMapElm = reflect.ValueOf(srcMapElm.Interface())
						if dstMapElm.IsValid() {
							dstMapElm = reflect.ValueOf(dstMapElm.Interface())
						}
					}
					if err = deepMerge(dstMapElm, srcMapElm, visited, depth+1, config); err != nil {
						return
					}
				case reflect.Slice:
					srcSlice := reflect.ValueOf(srcElement.Interface())

					var dstSlice reflect.Value
					if !dstElement.IsValid() || dstElement.IsNil() {
						dstSlice = reflect.MakeSlice(srcSlice.Type(), 0, srcSlice.Len())
					} else {
						dstSlice = reflect.ValueOf(dstElement.Interface())
					}

					if (!isEmptyValue(src) || overwriteWithEmptySrc || overwriteSliceWithEmptySrc) && (overwrite || isEmptyValue(dst)) && !config.AppendSlice && !sliceDeepCopy {
						if typeCheck && srcSlice.Type() != dstSlice.Type() {
							return fmt.Errorf("cannot override two slices with different type (%s, %s)", srcSlice.Type(), dstSlice.Type())
						}
						dstSlice = srcSlice
					} else if config.AppendSlice {
						if srcSlice.Type() != dstSlice.Type() {
							return fmt.Errorf("cannot append two slices with different type (%s, %s)", srcSlice.Type(), dstSlice.Type())
						}
						dstSlice = reflect.AppendSlice(dstSlice, srcSlice)
					} else if sliceDeepCopy {
						i := 0
						for ; i < srcSlice.Len() && i < dstSlice.Len(); i++ {
							srcElement := srcSlice.Index(i)
							dstElement := dstSlice.Index(i)

							if srcElement.CanInterface() {
								srcElement = reflect.ValueOf(srcElement.Interface())
							}
							if dstElement.CanInterface() {
								dstElement = reflect.ValueOf(dstElement.Interface())
							}

							if err = deepMerge(dstElement, srcElement, visited, depth+1, config); err != nil {
								return
							}
						}

					}
					dst.SetMapIndex(key, dstSlice)
				}
			}
			if dstElement.IsValid() && !isEmptyValue(dstElement) && (reflect.TypeOf(srcElement.Interface()).Kind() == reflect.Map || reflect.TypeOf(srcElement.Interface()).Kind() == reflect.Slice) {
				continue
			}

			if srcElement.IsValid() && ((srcElement.Kind() != reflect.Ptr && overwrite) || !dstElement.IsValid() || isEmptyValue(dstElement)) {
				if dst.IsNil() {
					dst.Set(reflect.MakeMap(dst.Type()))
				}
				dst.SetMapIndex(key, srcElement)
			}
		}
	case reflect.Slice:
		if !dst.CanSet() {
			break
		}
		if (!isEmptyValue(src) || overwriteWithEmptySrc || overwriteSliceWithEmptySrc) && (overwrite || isEmptyValue(dst)) && !config.AppendSlice && !sliceDeepCopy {
			dst.Set(src)
		} else if config.AppendSlice {
			if src.Type() != dst.Type() {
				return fmt.Errorf("cannot append two slice with different type (%s, %s)", src.Type(), dst.Type())
			}
			dst.Set(reflect.AppendSlice(dst, src))
		} else if sliceDeepCopy {
			for i := 0; i < src.Len() && i < dst.Len(); i++ {
				srcElement := src.Index(i)
				dstElement := dst.Index(i)
				if srcElement.CanInterface() {
					srcElement = reflect.ValueOf(srcElement.Interface())
				}
				if dstElement.CanInterface() {
					dstElement = reflect.ValueOf(dstElement.Interface())
				}

				if err = deepMerge(dstElement, srcElement, visited, depth+1, config); err != nil {
					return
				}
			}
		}
	case reflect.Ptr:
		fallthrough
	case reflect.Interface:
		if isReflectNil(src) {
			if overwriteWithEmptySrc && dst.CanSet() && src.Type().AssignableTo(dst.Type()) {
				dst.Set(src)
			}
			break
		}

		if src.Kind() != reflect.Interface {
			if dst.IsNil() || (src.Kind() != reflect.Ptr && overwrite) {
				if dst.CanSet() && (overwrite || isEmptyValue(dst)) {
					dst.Set(src)
				}
			} else if src.Kind() == reflect.Ptr {
				if err = deepMerge(dst.Elem(), src.Elem(), visited, depth+1, config); err != nil {
					return
				}
			} else if dst.Elem().Type() == src.Type() {
				if err = deepMerge(dst.Elem(), src, visited, depth+1, config); err != nil {
					return
				}
			} else {
				return ErrDifferentArgumentsTypes
			}
			break
		}

		if dst.IsNil() || overwrite {
			if dst.CanSet() && (overwrite || isEmptyValue(dst)) {
				dst.Set(src)
			}
			break
		}

		if dst.Elem().Kind() == src.Elem().Kind() {
			if err = deepMerge(dst.Elem(), src.Elem(), visited, depth+1, config); err != nil {
				return
			}
			break
		}
	default:
		mustSet := (isEmptyValue(dst) || overwrite) && (!isEmptyValue(src) || overwriteWithEmptySrc)
		if mustSet {
			if dst.CanSet() {
				dst.Set(src)
			} else {
				dst = src
			}
		}
	}

	return
}

// Merge will fill any empty for value type attributes on the dst struct using corresponding
// src attributes if they themselves are not empty. dst and src must be valid same-type structs
// and dst must be a pointer to struct.
// It won't merge unexported (private) fields and will do recursively any exported field.
func Merge(dst, src interface{}, opts ...func(*Config)) error {
	return merge(dst, src, opts...)
}

// MergeWithOverwrite will do the same as Merge except that non-empty dst attributes will be overridden by
// non-empty src attribute values.
// Deprecated: use Merge(…) with WithOverride
func MergeWithOverwrite(dst, src interface{}, opts ...func(*Config)) error {
	return merge(dst, src, append(opts, WithOverride)...)
}

// WithTransformers adds transformers to merge, allowing to customize the merging of some types.
func WithTransformers(transformers Transformers) func(*Config) {
	return func(config *Config) {
		config.Transformers = transformers
	}
}

// WithOverride will make merge override non-empty dst attributes with non-empty src attributes values.
func WithOverride(config *Config) {
	config.Overwrite = true
}

// WithOverwriteWithEmptyValue will make merge override non empty dst attributes with empty src attributes values.
func WithOverwriteWithEmptyValue(config *Config) {
	config.Overwrite = true
	config.overwriteWithEmptyValue = true
}

// WithOverrideEmptySlice will make merge override empty dst slice with empty src slice.
func WithOverrideEmptySlice(config *Config) {
	config.overwriteSliceWithEmptyValue = true
}

// WithAppendSlice will make merge append slices instead of overwriting it.
func WithAppendSlice(config *Config) {
	config.AppendSlice = true
}

// WithTypeCheck will make merge check types while overwriting it (must be used with WithOverride).
func WithTypeCheck(config *Config) {
	config.TypeCheck = true
}

// WithSliceDeepCopy will merge slice element one by one with Overwrite flag.
func WithSliceDeepCopy(config *Config) {
	config.sliceDeepCopy = true
	config.Overwrite = true
}

func merge(dst, src interface{}, opts ...func(*Config)) error {
	if dst != nil && reflect.ValueOf(dst).Kind() != reflect.Ptr {
		return ErrNonPointerAgument
	}
	var (
		vDst, vSrc reflect.Value
		err        error
	)

	config := &Config{}

	for _, opt := range opts {
		opt(config)
	}

	if vDst, vSrc, err = resolveValues(dst, src); err != nil {
		return err
	}
	if vDst.Type() != vSrc.Type() {
		return ErrDifferentArgumentsTypes
	}
	return deepMerge(vDst, vSrc, make(map[uintptr]*visit), 0, config)
}

// IsReflectNil is the reflect value provided nil
func isReflectNil(v reflect.Value) bool {
	k := v.Kind()
	switch k {
	case reflect.Interface, reflect.Slice, reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr:
		// Both interface and slice are nil if first word is 0.
		// Both are always bigger than a word; assume flagIndir.
		return v.IsNil()
	default:
		return false
	}
}

const (
	FieldTagName         string = `config`
	FieldTagOptional     string = `optional`
	FieldTagFinal        string = `final`
	FieldTagMustOverride string = `mustoverride`
)

// parseField inspects the metadata for a struct field and returns relevant values
// In particular, this is where struct tags are handled
func parseField(f reflect.StructField) FieldInfo {
	rtn := FieldInfo{
		Name: f.Name,
		Typ:  f.Type,
		Kind: f.Type.Kind(),
	}

	switch rtn.Kind {
	case reflect.Struct, reflect.Map, reflect.Array, reflect.Slice:
		rtn.Complex = true
	default:
		rtn.Complex = false
	}

	if v, ok := f.Tag.Lookup(FieldTagName); ok {
		rtn.Tags = strings.Split(v, ",")
		rtn.Optional = strings.Contains(v, FieldTagOptional)
		rtn.Final = strings.Contains(v, FieldTagFinal)
		rtn.Mustoverride = strings.Contains(v, FieldTagMustOverride)
	}

	return rtn
}

type FieldInfo struct {
	Name         string
	Tags         []string
	Typ          reflect.Type
	Kind         reflect.Kind
	Optional     bool
	Token        string
	Final        bool
	Complex      bool
	Mustoverride bool
}

// valueFromEnvironment checks the submitted environment variable name for a value
// if a value is found, the submitted value is overwritten with the value of the environment variable
// this only works for certain scalar types: string, int, bool, and float64
// returns true if and only if the value is overwritten
// If we can't find the environment variable with the name as submitted, then we tyry again
// with the name of the variable in ALL-CAPS.
// there is probably a very elegant way to handle the pointer case with a recursive call
// but I didn't feel like figuring it out after getting this here to work 8^)
func valueFromEnvironment(fieldValue reflect.Value, envVarName string) bool {
	// zero value
	// var z reflect.Value
	fieldType := fieldValue.Type()

	switch fieldType.Kind() {
	case reflect.Ptr:
		// this field is a pointer
		// if it is a pointer to a simple scalar type, we can check the environment for override variables
		pointedToType := fieldType.Elem()
		pointedToKind := fieldType.Elem().Kind()

		switch pointedToKind {
		case reflect.Bool:
			if envVal := getEnvironmentBool(envVarName); envVal != nil {
				if fieldValue.Elem().IsValid() {
					// the pointer in the base struct IS NOT NIL, so we can overwrite its target directly
					fieldValue.Elem().SetBool(*envVal)
					return true
				}
				// the pointer in the base struct IS NIL, so we can't address its target
				fieldValue.Set(reflect.ValueOf(envVal))
				return true
			}
		case reflect.Int:
			if envVal := getEnvironmentInt(envVarName); envVal != nil {
				if fieldValue.Elem().IsValid() {
					// the pointer in the base struct IS NOT NIL, so we can overwrite its target directly
					fieldValue.Elem().SetInt(int64(*envVal))
					return true
				}
				// the pointer in the base struct IS NIL, so we can't address its target
				fieldValue.Set(reflect.ValueOf(envVal))
				return true
			}
		case reflect.Float64:
			if envVal := getEnvironmentFloat64(envVarName); envVal != nil {
				if fieldValue.Elem().IsValid() {
					// the pointer in the base struct IS NOT NIL, so we can overwrite its target directly
					fieldValue.Elem().SetFloat(*envVal)
					return true
				}
				// the pointer in the base struct IS NIL, so we can't address its target
				fieldValue.Set(reflect.ValueOf(envVal))
				return true
			}
		case reflect.String:
			if envVal := getEnvironmentString(envVarName); envVal != nil {
				if pointedToType == reflect.TypeOf(`foo`) {
					// this is a regular string, so easy:
					if fieldValue.Elem().IsValid() {
						// the pointer in the base struct IS NOT NIL, so we can overwrite its target directly
						fieldValue.Elem().SetString(*envVal)
						return true
					}
					// the pointer in the base struct IS NIL, so we can't address its target
					fieldValue.Set(reflect.ValueOf(envVal))
					return true
				} else {
					envVal := reflect.ValueOf(*envVal)
					castedVal := envVal.Convert(pointedToType)
					if !fieldValue.Elem().IsValid() {
						// the pointer in the base struct IS NIL, so we have to set it
						empty := reflect.New(pointedToType)
						fieldValue = empty
					}
					// the pointer in the base struct IS NOT NIL, so we can overwrite its target directly
					fieldValue.Elem().Set(castedVal)
					return true
				}

			}
		default:
			// not a type we can work with
			// TODO: log an error
			//return z
			return false
		} // END pointer target kind switch

	case reflect.Bool:
		if envVal := getEnvironmentBool(envVarName); envVal != nil {
			fieldValue.SetBool(*envVal)
			return true
			//return reflect.ValueOf(*envVal)
		}
	case reflect.Int:
		if envVal := getEnvironmentInt(envVarName); envVal != nil {
			fieldValue.SetInt(int64(*envVal))
			return true
			//return reflect.ValueOf(*envVal)
		}
	case reflect.Float64:
		if envVal := getEnvironmentFloat64(envVarName); envVal != nil {
			fieldValue.SetFloat(*envVal)
			return true
			//return reflect.ValueOf(*envVal)
		}
	case reflect.String:
		if envVal := getEnvironmentString(envVarName); envVal != nil {
			fieldValue.SetString(*envVal)
			return true
			//return reflect.ValueOf(*envVal)
		}
	default:
		// not a type we can override with environment variables
		// TODO: log an error
		return false
	} // END field kind switch

	capECName := strings.ToUpper(envVarName)
	if capECName != envVarName {
		return valueFromEnvironment(fieldValue, capECName)
	}
	return false
}

func getEnvironmentString(vn string) *string {
	var rtn *string
	if value := os.Getenv(vn); value != "" {
		rtn = &value
	}

	return rtn
}

func getEnvironmentInt(vn string) *int {
	var rtn *int
	if strvalue := os.Getenv(vn); strvalue != "" {
		if value, err := strconv.Atoi(strvalue); err == nil {
			rtn = &value
		}
	}
	return rtn
}

func getEnvironmentBool(vn string) *bool {
	var rtn *bool
	if strvalue := os.Getenv(vn); strvalue != "" {
		if value, err := strconv.ParseBool(strvalue); err == nil {
			rtn = &value
		}
	}
	return rtn
}

func getEnvironmentFloat64(vn string) *float64 {
	var rtn *float64
	if strvalue := os.Getenv(vn); strvalue != "" {
		if value, err := strconv.ParseFloat(strvalue, 64); err == nil {
			rtn = &value
		}
	}
	return rtn
}
