// Package value provides helpers for the dynamic values that flow through
// workflows: scalars, map[string]any and []any.
package value

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// String converts v into the string used when it is embedded in a template.
// Strings are used as is; other values are encoded as JSON.
func String(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1e15 {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return String(float64(v))
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// Format renders v for diagnostics.
func Format(v any) string {
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	return String(v)
}

// FromJSON decodes JSON data. Integral numbers become int, others float64.
func FromJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected data after JSON value")
	}
	return normalize(v), nil
}

func normalize(v any) any {
	switch v := v.(type) {
	case json.Number:
		if i, err := strconv.ParseInt(string(v), 10, 0); err == nil {
			return int(i)
		}
		f, _ := v.Float64()
		return f
	case map[string]any:
		for k, e := range v {
			v[k] = normalize(e)
		}
	case []any:
		for i, e := range v {
			v[i] = normalize(e)
		}
	}
	return v
}

// IsNumber reports whether v is a Go numeric value.
func IsNumber(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

// ToFloat converts a numeric value to float64.
func ToFloat(v any) (float64, bool) {
	switch v := v.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

// TypeName returns the workflow-level type name of v.
func TypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "bool"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	}
	if IsNumber(v) {
		return "number"
	}
	return fmt.Sprintf("%T", v)
}
