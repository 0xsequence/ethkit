package ethgas

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseLatest(t *testing.T) {
	body := `{"fast": 100.0, "fastest": 200.0, "safeLow": 13.0, "average": 30.0, "block_time": 11.76923076923077, "blockNum": 8355683, "speed": 0.9989258227743392, "safeLowWait": 14.0, "avgWait": 1.9, "fastWait": 0.4, "fastestWait": 0.4}`
	want := StationLatest{Fast: 100, Fastest: 200, SafeLow: 13, Average: 30, BlockTime: 11.76923076923077, BlockNum: 8355683, Speed: 0.9989258227743392, SafeLowWait: 14, AvgWait: 1.9, FastWait: 0.4, FastestWait: 0.4}
	var have StationLatest
	err := json.Unmarshal([]byte(body), &have)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%# v", have)
	assert.Equal(t, want, have)
}

func TestStationLatest(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	s := &Station{
		Timeout: time.Second * 10,
		Debug:   true,
	}
	latest, err := s.Latest()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%# v", latest)
}

func TestParsePredictionTable(t *testing.T) {
	body := `[{"gasprice": 0.01, "hashpower_accepting": 97.9797979798, "hashpower_accepting2": 66.2337662338, "tx_atabove": 445.0, "age": null, "pct_remaining5m":25.0, "pct_mined_5m": 100.0, "total_seen_5m": 1.0, "average": 1050, "safelow": 100, "nomine": null, "avgdiff": 1, "intercept": 4.8015, "hpa_coef": -0.0243, "avgdiff_coef": -1.6459, "tx_atabove_coef": 0.0006, "int2": 6.9238, "hpa_coef2": -0.067, "sum": 6.9238, "expectedWait": 1000.0, "unsafe": 1, "expectedTime": 185.42}]`
	want := []*StationPrediction{
		&StationPrediction{GasPrice: 0.01, HashPowerAccepting: 97.9797979798, HashPowerAccepting2: 66.2337662338, TxAtAbove: 445, Age: nil, PctRemaining5m: 25, PctMined5m: 100, TotalSeen5m: 1, Average: 1050, SafeLow: 100, NoMine: nil, AvgDiff: 1, Intercept: 4.8015, HPACoef: -0.0243, AvgDiffCoef: -1.6459, TxAtAboveCoef: 0.0006, Int2: 6.9238, HPACoef2: -0.067, Sum: 6.9238, ExpectedWait: 1000, Unsafe: 1, ExpectedTime: 185.42},
	}
	var have []*StationPrediction
	err := json.Unmarshal([]byte(body), &have)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%# v", have[0])
	assert.Equal(t, want, have)
}

func TestStationPredictionTable(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	s := &Station{
		Timeout: time.Second * 10,
		Debug:   true,
	}
	predictions, err := s.PredictionTable()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%# v", predictions[0])
}
