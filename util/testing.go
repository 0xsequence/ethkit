package util

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
)

func ReadTestFile(t *testing.T) map[string]string {
	config := map[string]string{}
	testFile := "../ethkit-test.json"

	_, err := os.Stat(testFile)
	if err != nil {
		return config
	}

	data, err := ioutil.ReadFile("../ethkit-test.json")
	if err != nil {
		t.Fatalf("%s file could not be read", testFile)
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		t.Fatalf("%s file json parsing error", testFile)
	}

	return config
}
