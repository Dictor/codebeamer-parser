package main

import (
	"encoding/json"
	"os"
)

type JsonNode struct {
	Id    string `json:"id"`
	Label string `json:"label"`
}

type JsonEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type JsonGraph struct {
	Nodes map[string]JsonNode `json:"-"`
	Edges map[string]JsonEdge `json:"-"`
}

func NewJsonGraph() *JsonGraph {
	return &JsonGraph{
		Nodes: make(map[string]JsonNode),
		Edges: make(map[string]JsonEdge),
	}
}

func (g *JsonGraph) AddNode(id, label string) {
	if _, exists := g.Nodes[id]; !exists {
		g.Nodes[id] = JsonNode{Id: id, Label: label}
	}
}

func (g *JsonGraph) AddEdge(from, to string) {
	k := from + "->" + to
	if _, exists := g.Edges[k]; !exists {
		g.Edges[k] = JsonEdge{From: from, To: to}
	}
}

// MarshalJSON overriding for outputting internal maps as standard arrays
func (g *JsonGraph) MarshalJSON() ([]byte, error) {
	type Alias JsonGraph // prevents infinite recursion during unmarshaling

	orderedNodes := make([]JsonNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		orderedNodes = append(orderedNodes, n)
	}

	orderedEdges := make([]JsonEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		orderedEdges = append(orderedEdges, e)
	}

	return json.Marshal(&struct {
		Nodes []JsonNode `json:"nodes"`
		Edges []JsonEdge `json:"edges"`
	}{
		Nodes: orderedNodes,
		Edges: orderedEdges,
	})
}

// SaveGraphJSON saves the parsed generic JSON graph to local file
func SaveGraphJSON(graphData *JsonGraph) {
	Logger.Info("saving interactive graph UI as JSON")

	graphDataJSON, err := json.MarshalIndent(graphData, "", "  ")
	if err != nil {
		Logger.WithError(err).Error("Failed to marshal graph.json")
		return
	}

	jsonPath := "graph.json"
	if err := os.WriteFile(jsonPath, graphDataJSON, 0644); err != nil {
		Logger.WithError(err).Error("Failed to write graph.json to disk")
		return
	}
}
