package main

import (
	"fmt"
	"os"
	"testing"
)

// generateDummyGraph creates a hierarchical graph based on branching factors for each depth.
func generateDummyGraph(branchingFactors []int, addCrossLinks bool) *ExportGraph {
	graph := NewExportGraph()
	graph.AddNode("ROOT", "Massive Root Node")

	var allNodeIds []string
	nodeCounter := 0

	var recursiveAdd func(parentId string, depth int)
	recursiveAdd = func(parentId string, depth int) {
		if depth >= len(branchingFactors) {
			return
		}

		numChildren := branchingFactors[depth]
		for i := 0; i < numChildren; i++ {
			nodeId := fmt.Sprintf("NODE-%d", nodeCounter)
			label := fmt.Sprintf("Node L%d-%d", depth, nodeCounter)
			graph.AddNode(nodeId, label)
			graph.AddEdge(parentId, nodeId)
			allNodeIds = append(allNodeIds, nodeId)
			nodeCounter++

			recursiveAdd(nodeId, depth+1)
		}
	}

	recursiveAdd("ROOT", 0)

	// Simulate complex references (hyperlink linkages)
	if addCrossLinks && len(allNodeIds) > 10 {
		for i := 0; i < len(allNodeIds); i++ {
			if i%7 == 0 {
				source := allNodeIds[i]
				target := allNodeIds[(i+13)%len(allNodeIds)]
				graph.AddEdge(source, target)
			}
		}
	}

	return graph
}

// TestSaveGraphJSON_LargeGraph tests JSON generation with a large hierarchical graph.
func TestSaveGraphJSON_LargeGraph(t *testing.T) {
	// (200, 10, 5) results in 200 + (200*10) + (200*10*5) = 12,200 nodes
	jsonGraph := generateDummyGraph([]int{200, 10, 5}, true)

	SaveGraphJSON(jsonGraph)

	stat, err := os.Stat("graph.json")
	if err != nil {
		t.Fatalf("Failed to stat graph.json: %v", err)
	}

	sizeMB := float64(stat.Size()) / 1024.0 / 1024.0
	t.Logf("Generated graph.json size: %.2f MB", sizeMB)
}

// TestSaveGraphML_LargeGraph tests GraphML generation with a large hierarchical graph.
func TestSaveGraphML_LargeGraph(t *testing.T) {
	jsonGraph := generateDummyGraph([]int{200, 10, 5}, true)

	SaveGraphML(jsonGraph)

	stat, err := os.Stat("graph.graphml")
	if err != nil {
		t.Fatalf("Failed to stat graph.graphml: %v", err)
	}

	sizeMB := float64(stat.Size()) / 1024.0 / 1024.0
	t.Logf("Generated graph.graphml size: %.2f MB", sizeMB)
}
