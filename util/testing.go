package util

import (
	"fmt"
	"os"

	"github.com/bytedance/sonic"
)

func ReadTestConfig(testConfigFile string) (map[string]string, error) {
	config := map[string]string{}

	_, err := os.Stat(testConfigFile)
	if err != nil {
		return config, nil
	}

	data, err := os.ReadFile(testConfigFile)
	if err != nil {
		return nil, fmt.Errorf("%s file could not be read", testConfigFile)
	}

	err = sonic.ConfigFastest.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("%s file json parsing error", testConfigFile)
	}

	return config, nil
}
