package ethgas

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

// ETH Gas Station API - https://ethgasstation.info/
//
// Sample usage:
//
// station := &ethgas.Station{Timeout: time.Second * 10}
// latest, err := station.Latest()
// predictions, err := station.PredictionTable()
type Station struct {
	Timeout time.Duration
	Debug   bool
}

type StationLatest struct {
	Fast        float64
	Fastest     float64
	SafeLow     float64
	Average     float64
	BlockTime   float64 `json:"block_time"`
	BlockNum    int64
	Speed       float64
	SafeLowWait float64
	AvgWait     float64
	FastWait    float64
	FastestWait float64
}

func (s *Station) Latest() (*StationLatest, error) {
	r, err := http.NewRequest("GET", "https://ethgasstation.info/json/ethgasAPI.json", nil)
	if err != nil {
		return nil, err
	}
	w, err := s.fetch(r)
	if err != nil {
		return nil, err
	}
	var latest StationLatest
	err = s.parse(w, &latest)
	if err != nil {
		return nil, err
	}
	return &latest, nil
}

type StationPrediction struct {
	GasPrice            float64
	HashPowerAccepting  float64 `json:"hashpower_accepting"`
	HashPowerAccepting2 float64 `json:"hashpower_accepting2"`
	TxAtAbove           float64 `json:"tx_atabove"`
	Age                 interface{}
	PctRemaining5m      float64 `json:"pct_remaining5m"`
	PctMined5m          float64 `json:"pct_mined_5m"`
	TotalSeen5m         float64 `json:"total_seen_5m"`
	Average             float64
	SafeLow             float64
	NoMine              interface{}
	AvgDiff             float64
	Intercept           float64
	HPACoef             float64 `json:"hpa_coef"`
	AvgDiffCoef         float64 `json:"avgdiff_coef"`
	TxAtAboveCoef       float64 `json:"tx_atabove_coef"`
	Int2                float64
	HPACoef2            float64 `json:"hpa_coef2"`
	Sum                 float64
	ExpectedWait        float64
	Unsafe              int8
	ExpectedTime        float64
}

func (s *Station) PredictionTable() ([]*StationPrediction, error) {
	r, err := http.NewRequest("GET", "https://ethgasstation.info/json/predictTable.json", nil)
	if err != nil {
		return nil, err
	}
	w, err := s.fetch(r)
	if err != nil {
		return nil, err
	}
	var table []*StationPrediction
	err = s.parse(w, &table)
	if err != nil {
		return nil, err
	}
	return table, nil
}

func (s *Station) parse(w *http.Response, v interface{}) error {
	body, err := ioutil.ReadAll(w.Body)
	if err != nil {
		return err
	}
	if s.Debug {
		log.Printf("ETH Gas Station: %s", body)
	}
	return json.Unmarshal(body, v)
}

func (s *Station) fetch(r *http.Request) (*http.Response, error) {
	ctx, _ := context.WithTimeout(r.Context(), s.Timeout)
	return http.DefaultClient.Do(r.WithContext(ctx))
}
