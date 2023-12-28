package main

import (
	"fmt"
	"strconv"
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
				"item4": 2E7,
				"item5": 2.123456e7,
			},
		},
	}

	s        string
	rows     []string
	p        Printable
	minwidth = 24
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
	fmt.Println(s)
	rows = strings.Split(s, "\n")
}

func Test_Columnize(t *testing.T) {
	setup()
	for i := 0; i < len(rows); i++ {
		if rows[i] != "" {
			// the delimiter should be in the same position in all the rows
			assert.Equal(t, strings.Index(rows[i], "|"), minwidth)
			if strings.Contains(rows[i], "item5") {
				v := strconv.FormatFloat(complex.ObjList[1]["item5"].(float64), 'f', -1, 64)
				// the value of the nested object should be indented to the 3rd column and it should be parsed as an integer
				// left bound: 2*\t + 1*' ' = 3 , right bound: 3 + len(21234560) + 1 = 11
				assert.Equal(t, rows[i][minwidth*2+3:minwidth*2+11], v)
			}
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
