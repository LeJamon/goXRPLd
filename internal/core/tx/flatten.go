package tx

import (
	"reflect"
	"strings"
	"sync"
)

// flattenField holds pre-computed metadata for a single struct field.
type flattenField struct {
	index     int
	name      string
	omitempty bool
	isAmount  bool // use flattenAmount converter
	boolint   bool // convert bool to int (0 or 1)
}

// flattenInfo holds cached flatten metadata for a struct type.
type flattenInfo struct {
	fields []flattenField
}

// flattenCache stores pre-computed flattenInfo per type to avoid repeated reflection.
var flattenCache sync.Map // map[reflect.Type]*flattenInfo

// parseXRPLTag parses an xrpl struct tag.
// Format: "FieldName,opt1,opt2" where options are: omitempty, amount
// Returns ("", false) for tags that should be skipped ("-").
func parseXRPLTag(tag string) (name string, omitempty bool, isAmount bool, boolint bool, skip bool) {
	if tag == "" || tag == "-" {
		return "", false, false, false, true
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	for _, opt := range parts[1:] {
		switch opt {
		case "omitempty":
			omitempty = true
		case "amount":
			isAmount = true
		case "boolint":
			boolint = true
		}
	}
	return name, omitempty, isAmount, boolint, false
}

// getFlattenInfo returns the cached flattenInfo for a struct type.
// On first access, it computes and caches the info.
func getFlattenInfo(t reflect.Type) *flattenInfo {
	if cached, ok := flattenCache.Load(t); ok {
		return cached.(*flattenInfo)
	}

	info := &flattenInfo{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip embedded fields (like BaseTx)
		if field.Anonymous {
			continue
		}

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("xrpl")
		name, omitempty, isAmount, boolint, skip := parseXRPLTag(tag)
		if skip {
			continue
		}

		info.fields = append(info.fields, flattenField{
			index:     i,
			name:      name,
			omitempty: omitempty,
			isAmount:  isAmount,
			boolint:   boolint,
		})
	}

	flattenCache.Store(t, info)
	return info
}

// isEmptyValue returns true if the reflect.Value should be considered empty for omitempty.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.String:
		return v.String() == ""
	case reflect.Slice, reflect.Array:
		return v.Len() == 0
	case reflect.Map:
		return v.IsNil() || v.Len() == 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Struct:
		// For Amount structs, check if Value is empty
		if v.Type() == reflect.TypeOf(Amount{}) {
			valueField := v.FieldByName("Value")
			return valueField.String() == ""
		}
		return false
	default:
		return false
	}
}

// ReflectFlatten generates a flat map from a Transaction using struct tags.
// It starts with Common.ToMap() and adds type-specific fields based on xrpl tags.
func ReflectFlatten(tx Transaction) (map[string]any, error) {
	m := tx.GetCommon().ToMap()

	v := reflect.ValueOf(tx)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	info := getFlattenInfo(v.Type())

	for _, f := range info.fields {
		val := v.Field(f.index)

		if f.omitempty && isEmptyValue(val) {
			continue
		}

		if f.isAmount {
			// Handle Amount and *Amount
			var amt Amount
			if val.Kind() == reflect.Ptr {
				if val.IsNil() {
					continue
				}
				amt = val.Elem().Interface().(Amount)
			} else {
				amt = val.Interface().(Amount)
			}
			m[f.name] = flattenAmount(amt)
		} else if f.boolint {
			// Convert bool to int (0 or 1) for XRPL protocol
			if val.Bool() {
				m[f.name] = 1
			} else {
				m[f.name] = 0
			}
		} else {
			// Default: dereference pointers
			if val.Kind() == reflect.Ptr {
				if val.IsNil() {
					continue
				}
				m[f.name] = val.Elem().Interface()
			} else {
				m[f.name] = val.Interface()
			}
		}
	}

	return m, nil
}

// flattenAmount converts an Amount to its serializable form.
// Native XRP amounts are returned as a string value.
// Issued currency amounts are returned as a map with value/currency/issuer.
// Note: The binary codec expects map[string]any, not map[string]string.
func flattenAmount(a Amount) any {
	if a.IsNative() {
		return a.Value()
	}
	return map[string]any{
		"value":    a.Value(),
		"currency": a.Currency,
		"issuer":   a.Issuer,
	}
}
