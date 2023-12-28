package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	complex = struct {
		Name    string           `json:"name"`
		List    []any            `json:"list"`
		Nested  Nested           `json:"nested"`
		Object  map[string]any   `json:"object"`
		ObjList []map[string]any `json:"objList"`
	}{
		Name: "complex",
		List: []any{
			"first",
			"second",
		},
		Nested: Nested{
			Title: "hello",
			Value: 500000000,
		},
		Object: map[string]any{
			"obj1": 1,
			"obj2": -2,
		},
		ObjList: []map[string]any{
			{
				"item1": 1,
				"item2": 2,
			},
			{
				"item3": 3e7,
				"item4": 2e7,
				"item5": 2.123456e7,
			},
		},
	}

	s        string
	rows     []string
	p        Printable
	minwidth = 20
	tabwidth = 0
	padding  = 0
	padchar  = byte(' ')
)

type Nested struct {
	Title string `json:"title"`
	Value uint   `json:"value"`
}

func setup() {
	if err := p.FromStruct(complex); err != nil {
		panic(err)
	}
	s = p.Columnize(*NewPrintableFormat(minwidth, tabwidth, padding, padchar))
	rows = strings.Split(s, "\n")
}

func Test_Columnize(t *testing.T) {
	setup()
	for i := 0; i < len(rows); i++ {
		if rows[i] != "" {
			// the delimiter should be in the same position in all the rows
			assert.Equal(t, strings.Index(rows[i], "|"), minwidth)
		}
	}
}

func Test_GetValueByJSONTag(t *testing.T) {
	setup()
	tag := "title"
	assert.Equal(t, GetValueByJSONTag(complex, tag), complex.Nested.Title)
}

func Test_GetValueByJSONTag_FailWhenNotStruct(t *testing.T) {
	setup()
	tag := "title"
	assert.Nil(t, GetValueByJSONTag(p, tag))
}
