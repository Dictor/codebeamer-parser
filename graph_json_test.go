package main

import (
	"fmt"
	"os"
	"testing"
)

// TestSaveGraphJSON_LargeGraph creates a massive artificial graph and tests the JSON generation performance.
func TestSaveGraphJSON_LargeGraph(t *testing.T) {
	// Create a dummy JSON graph
	jsonGraph := NewJsonGraph()

	const numTrackers = 500
	const numIssuesPerTracker = 30

	issueCounter := 0
	jsonGraph.AddNode("ROOT", "Massive Root Tracker")

	for i := 0; i < numTrackers; i++ {
		trackerId := fmt.Sprintf("TRACKER-%d", i)
		jsonGraph.AddNode(trackerId, fmt.Sprintf("Tracker Node %d", i))
		jsonGraph.AddEdge("ROOT", trackerId)

		for j := 0; j < numIssuesPerTracker; j++ {
			issueId := fmt.Sprintf("ISSUE-%d", issueCounter)
			jsonGraph.AddNode(issueId, fmt.Sprintf("Issue Node %d", issueCounter))
			jsonGraph.AddEdge(trackerId, issueId)
			issueCounter++

			// Add some random fake links mapping to simulate complex edges
			if issueCounter > 10 && issueCounter%5 == 0 {
				targetId := fmt.Sprintf("ISSUE-%d", issueCounter-7)
				jsonGraph.AddEdge(issueId, targetId)
			}
		}
	}

	SaveGraphJSON(jsonGraph)

	// Check if graph.json was created
	stat, err := os.Stat("graph.json")
	if err != nil {
		t.Fatalf("Failed to stat graph.json: %v", err)
	}

	sizeMB := float64(stat.Size()) / 1024.0 / 1024.0
	t.Logf("Generated graph.json size: %.2f MB", sizeMB)

	if sizeMB < 0.2 {
		t.Logf("Warning: Graph size is smaller than expected. Currently: %.2f MB", sizeMB)
	}
}
