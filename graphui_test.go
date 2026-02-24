package main

import (
	"fmt"
	"os"
	"testing"
)

// TestSaveAndOpenGraphHTML_LargeGraph creates a massive artificial graph and tests the HTML generation performance.
func TestSaveAndOpenGraphHTML_LargeGraph(t *testing.T) {
	// Create a dummy root tracker
	rootTracker := &RootTrackerNode{
		Tracker: Tracker{
			Id:   "ROOT",
			Text: "Massive Root Tracker",
		},
	}

	// Calculate a large number of nodes to reach ~4MB HTML size.
	// Roughly, each node is about 60-80 bytes in JSON, and each edge is about 40 bytes.
	// 4MB / 120 bytes = ~33,000 nodes.

	const numTrackers = 100
	const numIssuesPerTracker = 300

	validChildTracker := make([]*TrackerNode, 0, numTrackers)
	linkRefs := make(map[string][]string)

	issueCounter := 0

	for i := 0; i < numTrackers; i++ {
		trackerId := fmt.Sprintf("TRACKER-%d", i)
		tracker := &TrackerNode{
			Tracker: Tracker{
				Id:        trackerId,
				TrackerId: i,
				Text:      fmt.Sprintf("Tracker Node %d", i),
			},
			Children: make([]*IssueNode, 0, numIssuesPerTracker),
		}

		for j := 0; j < numIssuesPerTracker; j++ {
			issueId := fmt.Sprintf("ISSUE-%d", issueCounter)
			issue := &IssueNode{
				Id:    issueId,
				Title: fmt.Sprintf("Issue Node %d", issueCounter),
			}
			tracker.Children = append(tracker.Children, issue)
			issueCounter++

			// Add some random fake links mapping to simulate complex edges
			if issueCounter > 10 && issueCounter%5 == 0 {
				targetId := fmt.Sprintf("ISSUE-%d", issueCounter-7)
				linkRefs[issueId] = append(linkRefs[issueId], targetId)
			}
		}

		validChildTracker = append(validChildTracker, tracker)
	}

	// 브라우저 팝업 생략을 위해 false 전달
	SaveAndOpenGraphHTML(rootTracker, validChildTracker, linkRefs, false)

	// Check if graph.html was created
	stat, err := os.Stat("graph.html")
	if err != nil {
		t.Fatalf("Failed to stat graph.html: %v", err)
	}

	sizeMB := float64(stat.Size()) / 1024.0 / 1024.0
	t.Logf("Generated graph.html size: %.2f MB", sizeMB)

	if sizeMB < 1.0 {
		t.Logf("Warning: Graph size is smaller than expected 1MB. Currently: %.2f MB", sizeMB)
	}
}
