package httpvcr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
)

type cassette struct {
	name     string
	Episodes []episode
}

type episode struct {
	Request  *vcrRequest
	Response *vcrResponse
}

func (c *cassette) Name() string {
	return c.name
}

func (c *cassette) Filename() string {
	return "fixtures/vcr/" + c.name + ".json"
}

func (c *cassette) Exists() bool {
	_, err := os.Stat(c.Filename())
	return err == nil
}

func (c *cassette) read() {
	jsonData, _ := ioutil.ReadFile(c.Filename())
	err := json.Unmarshal(jsonData, c)
	if err != nil {
		panic("httpvcr: cannot parse json!")
	}
}

func (c *cassette) write() {
	jsonData, _ := json.Marshal(c)

	var jsonOut bytes.Buffer
	json.Indent(&jsonOut, jsonData, "", "  ")

	os.MkdirAll("fixtures/vcr", 0755)
	err := ioutil.WriteFile(c.Filename(), jsonOut.Bytes(), 0644)
	if err != nil {
		panic("httpvcr: cannot write cassette file!")
	}
}

func (c *cassette) matchEpisode(request *vcrRequest) *episode {
	if len(c.Episodes) == 0 {
		panic("httpvcr: no more episodes!")
	}

	e := c.Episodes[0]
	expected := e.Request

	if expected.Method != request.Method {
		panicEpisodeMismatch(request, "Method", expected.Method, request.Method)
	}

	if expected.URL != request.URL {
		panicEpisodeMismatch(request, "URL", expected.URL, request.URL)
	}

	if !reflect.DeepEqual(expected.Body, request.Body) {
		panicEpisodeMismatch(request, "Body", string(expected.Body[:]), string(request.Body[:]))
	}

	c.Episodes = c.Episodes[1:]
	return &e
}

func panicEpisodeMismatch(request *vcrRequest, field string, expected string, actual string) {
	panic(fmt.Sprintf(
		"httpvcr: problem with episode for %s %s\n  episode %s does not match:\n  expected: %s\n  but got: %s",
		request.Method,
		request.URL,
		field,
		expected,
		actual,
	))
}
