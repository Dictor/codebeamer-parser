package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"text/template"
)

// VisNode represents a node in the vis.js network
type VisNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Title string `json:"title"` // HTML tooltip
	Group string `json:"group"` // Used for clustering/coloring
	Shape string `json:"shape"`
}

// VisEdge represents an edge in the vis.js network
type VisEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Arrows string `json:"arrows"`
}

// SaveAndOpenGraphHTML 루트 노드와 하위 트래커들을 분석하여 Vis.js의 클러스터링 기반 인터랙티브 HTML 뷰를 로컬에 저장하고 기본 브라우저를 통해 엽니다.
func SaveAndOpenGraphHTML(rootTracker *RootTrackerNode, vaildChildTracker []*TrackerNode, linkRefs map[string][]string) {
	Logger.Info("generating vis.js graph payload")

	var nodes []VisNode
	var edges []VisEdge

	// 트래커/이슈 노드를 재귀적으로 Vis.js 노드 배열에 추가
	var addTrackerToVis func(tracker *TrackerNode)
	var addIssueToVis func(issue *IssueNode, parentID string, group string)

	addIssueToVis = func(issue *IssueNode, parentID string, group string) {
		nodes = append(nodes, VisNode{
			ID:    issue.Id,
			Label: EscapeDotString(issue.Id),
			Title: EscapeDotString(fmt.Sprintf("[%s] %s", issue.Id, issue.Title)),
			Group: group,
			Shape: "box",
		})
		edges = append(edges, VisEdge{
			From:   parentID,
			To:     issue.Id,
			Arrows: "to",
		})

		for _, child := range issue.RealChildren {
			addIssueToVis(child, issue.Id, group)
		}
	}

	addTrackerToVis = func(tracker *TrackerNode) {
		groupName := fmt.Sprintf("tracker_%d", tracker.TrackerId)
		nodes = append(nodes, VisNode{
			ID:    tracker.Id,
			Label: EscapeDotString(tracker.Id),
			Title: EscapeDotString(fmt.Sprintf("[%s] %s", tracker.Id, tracker.Text)),
			Group: groupName,
			Shape: "ellipse",
		})
		edges = append(edges, VisEdge{
			From:   rootTracker.Id,
			To:     tracker.Id,
			Arrows: "to",
		})

		for _, issue := range tracker.Children {
			addIssueToVis(issue, tracker.Id, groupName)
		}
	}

	// 1. 루트 추가
	nodes = append(nodes, VisNode{
		ID:    rootTracker.Id,
		Label: EscapeDotString(rootTracker.Id),
		Title: EscapeDotString(rootTracker.Text),
		Group: "root",
		Shape: "database",
	})

	// 2. 하위 트래커 및 이슈 추가
	for _, childTracker := range vaildChildTracker {
		addTrackerToVis(childTracker)
	}

	// 3. 하이퍼링크 기반 참조 엣지 추가 (complexity 계산 로직에서 발견된 관계들)
	for fromID, toIDs := range linkRefs {
		for _, toID := range toIDs {
			edges = append(edges, VisEdge{
				From:   fromID,
				To:     toID,
				Arrows: "to",
			})
		}
	}

	nodesJSON, _ := json.Marshal(nodes)
	edgesJSON, _ := json.Marshal(edges)

	// Vis.js 렌더링을 위한 HTML 템플릿
	htmlTemplate := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Codebeamer Interactive Graph</title>
    <!-- vis-network CDN -->
    <script type="text/javascript" src="https://unpkg.com/vis-network/standalone/umd/vis-network.min.js"></script>
    <style type="text/css">
        body, html { font-family: sans-serif; height: 100%; margin: 0; padding: 0; overflow: hidden; }
        #network { width: 100%; height: 100%; border: none; }
        #loading { position: absolute; top: 50%; left: 50%; transform: translate(-50%, -50%); font-size: 24px; font-weight: bold; background: rgba(255,255,255,0.8); padding: 20px; border-radius: 10px; z-index: 100; }
        #layoutSelector { position: absolute; top: 20px; left: 20px; z-index: 100; background: white; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
    </style>
</head>
<body>
<div id="loading">Physics Simulation Running... Please wait</div>
<div id="layoutSelector">
    <label for="layout"><strong>Graph Layout: </strong></label>
    <select id="layout" onchange="changeLayout(this.value)">
        <option value="force">Force-Directed (Physics)</option>
        <option value="hierarchical_UD">Hierarchical (Up-Down Tree)</option>
        <option value="hierarchical_DU">Hierarchical (Down-Up Tree)</option>
        <option value="hierarchical_LR">Hierarchical (Left-Right Tree)</option>
        <option value="hierarchical_RL">Hierarchical (Right-Left Tree)</option>
    </select>
</div>
<div id="network"></div>
<script type="text/javascript">
    var nodes = new vis.DataSet({{.Nodes}});
    var edges = new vis.DataSet({{.Edges}});

    var container = document.getElementById('network');
    var data = { nodes: nodes, edges: edges };
    var options = {
        nodes: {
            shape: 'box',
            font: { size: 14 }
        },
        edges: {
            smooth: { type: 'continuous' }
        },
        physics: {
            enabled: true,
            barnesHut: {
                gravitationalConstant: -2000,
                centralGravity: 0.3,
                springLength: 95,
                springConstant: 0.04,
                damping: 0.09,
                avoidOverlap: 0.1
            },
            stabilization: {
                enabled: true,
                iterations: 1000,
                updateInterval: 100
            }
        },
        layout: { improvedLayout: false },
        interaction: { hover: true, tooltipDelay: 200, navigationButtons: true, keyboard: true }
    };

    var network = new vis.Network(container, data, options);

    // 레이아웃 변경 핸들러
    window.changeLayout = function(val) {
        if (val === 'force') {
            network.setOptions({
                layout: { hierarchical: false },
                physics: { enabled: true }
            });
        } else if (val.startsWith('hierarchical_')) {
            var direction = val.split('_')[1]; // UD, DU, LR, RL
            network.setOptions({
                layout: {
                    hierarchical: {
                        enabled: true,
                        direction: direction,
                        sortMethod: 'directed',
                        nodeSpacing: 150,
                        levelSeparation: 150
                    }
                },
                physics: { enabled: false } // 계층형 레이아웃에서는 기본적으로 물리 연산을 끔
            });
        }
    };

    // 물리 엔진 시뮬레이션 상태 표시
    network.on("stabilizationProgress", function(params) {
        document.getElementById('loading').innerText = "Physics Simulation Running... " + Math.round((params.iterations/params.total)*100) + "%";
    });
    network.once("stabilizationIterationsDone", function() {
        document.getElementById('loading').style.display = 'none';
        network.setOptions({ physics: { enabled: false } }); // 렌더링 속도를 위해 초기 큰 이동 후 물리 끄기
    });
	network.on("dragStart", function(params) {
		if (params.nodes.length > 0) { network.setOptions({ physics: { enabled: true } }); }
	});
	network.on("dragEnd", function(params) {
		network.setOptions({ physics: { enabled: false } });
	});

	// --- 줌 레벨에 따른 구조적 자동 클러스터링(POI 반응형) ---
	var isClustered = false;

	function clusterAllTrackers() {
		// 트래커 별 위계가 아닌, 그래프 구조(외곽의 고립된 노드들이나 허브 노드를 기준)에 따라 클러스터링
		
		var clusterOptionsByData = {
			processProperties: function(clusterOptions, childNodes) {
				clusterOptions.mass = 1;
				clusterOptions.label = 'Cluster ' + childNodes.length + ' nodes';
				clusterOptions.shape = 'database';
				clusterOptions.color = { background: '#ffcc00', border: '#ff9900' };
				clusterOptions.font = { size: 24, face: 'arial', bold: true };
				return clusterOptions;
			}
		};

		// 1단계: 주변에 엣지가 1개 뿐인 잔가지 노드(Outliers)들을 묶음
		network.clusterOutliers(clusterOptionsByData);

		// 2단계: 엣지가 많은 허브(Hub) 노드들을 기준으로 주변 노드를 합병함 (Hub Size = 허브로 간주할 최소 연결선 수)
		var hubsize = 5; 
		network.clusterByHubsize(hubsize, clusterOptionsByData);
	}

	function openAllClusters() {
		Object.keys(network.body.nodes).forEach(function(nodeId) {
			if (network.isCluster(nodeId)) {
				network.openCluster(nodeId);
			}
		});

		// 클러스터에서 풀려난 노드가 겹쳐있지 않고 부드럽게 펼쳐지도록 물리 엔진을 잠시 켰다가 끕니다.
		network.setOptions({ physics: { enabled: true } });
		setTimeout(function() {
			network.setOptions({ physics: { enabled: false } });
		}, 1500); // 1.5초 후 다시 물리 엔진 끄기
	}

	network.on("zoom", function (params) {
		var scale = network.getScale();
		var threshold = 0.4; // 줌 아웃 임계값 (작을수록 멀리서 보는 것)

		if (scale < threshold && !isClustered) {
			clusterAllTrackers();
			isClustered = true;
		} else if (scale >= threshold && isClustered) {
			openAllClusters();
			isClustered = false;
		}
	});

	// 개별 클러스터 노드를 더블 줌인 아웃 상관없이 더블 클릭해서 열고 닫을 수 있게 설정
	network.on("doubleClick", function(params) {
		if (params.nodes.length == 1) {
			if (network.isCluster(params.nodes[0]) == true) {
				network.openCluster(params.nodes[0]);
				// 클러스터 해제 시 물리 엔진 작동
				network.setOptions({ physics: { enabled: true } });
				setTimeout(function() {
					network.setOptions({ physics: { enabled: false } });
				}, 1500);
			}
		}
	});
</script>
</body>
</html>
`

	tmpl, err := template.New("graph").Parse(htmlTemplate)
	if err != nil {
		Logger.WithError(err).Error("Failed to parse HTML template")
		return
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]interface{}{
		"Nodes": string(nodesJSON),
		"Edges": string(edgesJSON),
	})
	if err != nil {
		Logger.WithError(err).Error("Failed to render HTML template")
		return
	}

	// HTML 파일로 기록
	Logger.Info("saving interactive graph UI as graph.html")
	htmlPath := "graph.html"
	if err := os.WriteFile(htmlPath, buf.Bytes(), 0644); err != nil {
		Logger.WithError(err).Error("Failed to write graph.html to disk")
		return
	}

	// 기본 브라우저를 통해 HTML 파일 열기
	Logger.Info("launching default OS browser to open graph.html")
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Windows: rundll32 url.dll,FileProtocolHandler 나 cmd /c start 사용 가능
		cmd = exec.Command("cmd", "/c", "start", htmlPath)
	case "darwin":
		cmd = exec.Command("open", htmlPath)
	default:
		// Linux: xdg-open
		cmd = exec.Command("xdg-open", htmlPath)
	}

	if err := cmd.Start(); err != nil {
		Logger.WithError(err).Error("Failed to open browser, please check graph.html manually.")
	}
}
