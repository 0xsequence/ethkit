package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"text/tabwriter"
)

type printableFormat struct {
	minwidth int
	tabwidth int
	padding  int
	padchar  byte
}

// NewPrintableFormat returns a customized configuration format
func NewPrintableFormat(minwidth, tabwidth, padding int, padchar byte) *printableFormat {
	return &printableFormat{minwidth, tabwidth, padding, padchar}
}

// Printable is a generic key-value (map) structure that could contain nested objects.
type Printable map[string]any

// PrettyJSON prints an object in "prettified" JSON format
func PrettyJSON(toJSON any) (*string, error) {
	b, err := json.MarshalIndent(toJSON, "", "  ")
	if err != nil {
		return nil, err
	}
	jsonString := string(b)

	// remove the trailing newline character ("%")
	if jsonString[len(jsonString)-1] == '\n' {
		jsonString = jsonString[:len(jsonString)-1]
	}

	return &jsonString, nil
}

// FromStruct converts a struct into a Printable using, when available, JSON field names as keys
func (p *Printable) FromStruct(input any) error {
	bytes, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, &p); err != nil {
		return err
	}

	return nil
}

// Columnize returns a formatted-in-columns (vertically aligned) string based on a provided configuration.
func (p *Printable) Columnize(pf printableFormat) string {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, pf.minwidth, pf.tabwidth, pf.padding, pf.padchar, tabwriter.Debug)
	// NOTE: Order is not maintained whilst looping over map's . Results from different execution may differ.
	for k, v := range *p {
		printKeyValue(w, k, v)
	}
	w.Flush()

	return buf.String()
}

func printKeyValue(w *tabwriter.Writer, key string, value any) {
	switch t := value.(type) {
	// NOTE: Printable is not directly inferred as map[string]any therefore explicit reference is necessary
	case map[string]any:
		fmt.Fprintln(w, key, "\t")
		for tk, tv := range t {
			printKeyValue(w, "\t "+tk, tv)
		}
	case []any:
		fmt.Fprintln(w, key, "\t")
		for _, elem := range t {
			elemMap, ok := elem.(map[string]any)
			if ok {
				for tk, tv := range elemMap {
					printKeyValue(w, "\t "+tk, tv)
				}
				fmt.Fprintln(w, "\t", "\t")
			} else {
				fmt.Fprintln(w, "\t", customFormat(elem))
			}
		}
	default:
		// custom format for numbers to avoid scientific notation
		fmt.Fprintf(w, "%s\t %s\n", key, customFormat(value))
	}
}

func customFormat(value any) string {
	switch v := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return formatFloat(v)
	default:
		return fmt.Sprintf("%v", base64ToHex(value))
	}
}

func formatFloat(f any) string {
	str := fmt.Sprintf("%v", f)
	if strings.ContainsAny(str, "eE.") {
		if floatValue, err := strconv.ParseFloat(str, 64); err == nil {
			return strconv.FormatFloat(floatValue, 'f', -1, 64)
		}
	}

	return str
}

func base64ToHex(str any) any {
	_, ok := str.(string); if !ok {
		return str
	}
	decoded, err := base64.StdEncoding.DecodeString(str.(string))
	if err != nil {
		return str
	}

	return "0x" + hex.EncodeToString(decoded)
}

// GetValueByJSONTag returns the value of a struct field matching a JSON tag provided in input.
func GetValueByJSONTag(input any, jsonTag string) any {
	// TODO: Refactor to support both nil values and errors when key not found
	return findField(reflect.ValueOf(input), jsonTag)
}

func findField(val reflect.Value, jsonTag string) any {
	seen := make(map[uintptr]bool)

	// take the value the pointer val points to
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	// return if the element is not a struct
	if val.Kind() != reflect.Struct {
		return nil
	}

	// check if the struct has already been processed to avoid infinite recursion
	if val.CanAddr() {
		ptr := val.Addr().Pointer()
		if seen[ptr] {
			return nil
		}
		seen[ptr] = true
	}

	t := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")

		fieldValue := val.Field(i)
		if fieldValue.Kind() == reflect.Struct {
			// recursively process fields including embedded ones
			return findField(fieldValue, jsonTag)
		} else {
			if strings.EqualFold(strings.ToLower(tag), strings.ToLower(jsonTag)) {
				return val.Field(i).Interface()
			}
		}
	}

	return nil
}
