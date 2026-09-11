package main

import (
	"encoding/json"
	"math"
	"testing"
)

func TestSnapshotProducesValidJSON(t *testing.T) {
	snapshot := Snapshot{Torrents: []TorrentView{{
		DownloadRate: jsonNumber(math.Inf(1)),
		UploadRate:   jsonNumber(math.NaN()),
		History:      []float64{jsonNumber(math.Inf(-1))},
		PeerList:     []PeerView{{DownloadRate: jsonNumber(math.Inf(1))}},
	}}}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Wails would return an empty JSON response: %v", err)
	}
	if !json.Valid(payload) {
		t.Fatal("snapshot JSON is invalid")
	}
}
