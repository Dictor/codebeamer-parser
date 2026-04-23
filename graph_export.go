package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
)

type GraphML struct {
	XMLName        xml.Name `xml:"graphml"`
	Xmlns          string   `xml:"xmlns,attr"`
	XmlnsY         string   `xml:"xmlns:y,attr"`
	XmlnsYed       string   `xml:"xmlns:yed,attr"`
	XmlnsXsi       string   `xml:"xmlns:xsi,attr"`
	SchemaLocation string   `xml:"xsi:schemaLocation,attr"`
	Key            GraphKey `xml:"key"`
	Graph          Graph    `xml:"graph"`
}

type GraphKey struct {
	ID        string `xml:"id,attr"`
	For       string `xml:"for,attr"`
	YFileType string `xml:"yfiles.type,attr"`
}

type Graph struct {
	ID          string      `xml:"id,attr"`
	EdgeDefault string      `xml:"edgedefault,attr"`
	Nodes       []GraphNode `xml:"node"`
	Edges       []GraphEdge `xml:"edge"`
}

type GraphNode struct {
	ID   string   `xml:"id,attr"`
	Data NodeData `xml:"data"`
}

type NodeData struct {
	Key       string    `xml:"key,attr"`
	ShapeNode ShapeNode `xml:"y:ShapeNode"`
}

type ShapeNode struct {
	Label string `xml:"y:NodeLabel"`
}

type GraphEdge struct {
	ID     string `xml:"id,attr"`
	Source string `xml:"source,attr"`
	Target string `xml:"target,attr"`
}

// SaveGraphML saves the graph as a GraphML file compatible with yEd
func SaveGraphML(graphData *ExportGraph) {
	gml := GraphML{
		Xmlns:          "http://graphml.graphdrawing.org/xmlns",
		XmlnsY:         "http://www.yworks.com/xml/graphml",
		XmlnsYed:       "http://www.yworks.com/xml/yed/3",
		XmlnsXsi:       "http://www.w3.org/2001/XMLSchema-instance",
		SchemaLocation: "http://graphml.graphdrawing.org/xmlns http://www.yworks.com/xml/schema/graphml/1.1/ygraphml.xsd",
		Key: GraphKey{
			ID:        "d6",
			For:       "node",
			YFileType: "nodegraphics",
		},
		Graph: Graph{
			ID:          "G",
			EdgeDefault: "directed",
		},
	}

	for _, n := range graphData.Nodes {
		gml.Graph.Nodes = append(gml.Graph.Nodes, GraphNode{
			ID: n.Id,
			Data: NodeData{
				Key: "d6",
				ShapeNode: ShapeNode{
					Label: n.Label,
				},
			},
		})
	}

	edgeIdx := 0
	for _, e := range graphData.Edges {
		gml.Graph.Edges = append(gml.Graph.Edges, GraphEdge{
			ID:     fmt.Sprintf("e%d", edgeIdx),
			Source: e.From,
			Target: e.To,
		},
		)
		edgeIdx++
	}

	output, err := xml.MarshalIndent(gml, "", "  ")
	if err != nil {
		Logger.WithError(err).Error("Failed to marshal graph.graphml")
		return
	}

	header := []byte(xml.Header)
	fullOutput := append(header, output...)

	if err := os.WriteFile("graph.graphml", fullOutput, 0644); err != nil {
		Logger.WithError(err).Error("Failed to write graph.graphml to disk")
	}
}

type ExportNode struct {
	Id    string `json:"id"`
	Label string `json:"label"`
}

type ExportEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ExportGraph struct {
	Nodes map[string]ExportNode `json:"-"`
	Edges map[string]ExportEdge `json:"-"`
}

func NewExportGraph() *ExportGraph {
	return &ExportGraph{
		Nodes: make(map[string]ExportNode),
		Edges: make(map[string]ExportEdge),
	}
}

func (g *ExportGraph) AddNode(id, label string) {
	if _, exists := g.Nodes[id]; !exists {
		g.Nodes[id] = ExportNode{Id: id, Label: label}
	}
}

func (g *ExportGraph) AddEdge(from, to string) {
	k := from + "->" + to
	if _, exists := g.Edges[k]; !exists {
		g.Edges[k] = ExportEdge{From: from, To: to}
	}
}

// MarshalJSON overriding for outputting internal maps as standard arrays
func (g *ExportGraph) MarshalJSON() ([]byte, error) {
	orderedNodes := make([]ExportNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		orderedNodes = append(orderedNodes, n)
	}

	orderedEdges := make([]ExportEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		orderedEdges = append(orderedEdges, e)
	}

	return json.Marshal(&struct {
		Nodes []ExportNode `json:"nodes"`
		Edges []ExportEdge `json:"edges"`
	}{
		Nodes: orderedNodes,
		Edges: orderedEdges,
	})
}

// SaveGraphJSON saves the parsed generic JSON graph to local file
func SaveGraphJSON(graphData *ExportGraph) {
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
