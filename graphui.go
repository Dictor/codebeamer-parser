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

// FGNode represents a node in the Force-Graph network
type FGNode struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group,omitempty"`
	Color string `json:"color,omitempty"`
	Val   int    `json:"val"`
}

// FGLink represents an edge in the Force-Graph network
type FGLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// FGData represents the entire graph structure
type FGData struct {
	Nodes []FGNode `json:"nodes"`
	Links []FGLink `json:"links"`
}

// SaveAndOpenGraphHTML 루트 노드와 하위 트래커들을 분석하여 Vis.js의 클러스터링 기반 인터랙티브 HTML 뷰를 로컬에 저장하고 기본 브라우저를 통해 엽니다.
// openBrowser가 참일 경우 파일 저장 후 기본 브라우저를 통해 엽니다.
func SaveAndOpenGraphHTML(rootTracker *RootTrackerNode, vaildChildTracker []*TrackerNode, linkRefs map[string][]string, openBrowser bool) {
	Logger.Info("generating vis.js graph payload")

	var nodes []FGNode
	var links []FGLink

	// 트래커/이슈 노드를 재귀적으로 Force-Graph 노드 배열에 추가
	var addTrackerToFG func(tracker *TrackerNode)
	var addIssueToFG func(issue *IssueNode, parentID string, group string)

	addIssueToFG = func(issue *IssueNode, parentID string, group string) {
		nodes = append(nodes, FGNode{
			ID:    issue.Id,
			Name:  EscapeDotString(fmt.Sprintf("[%s] %s", issue.Id, issue.Title)),
			Group: group,
			Color: "#0074D9",
			Val:   1, // 기본 크기
		})
		links = append(links, FGLink{
			Source: parentID,
			Target: issue.Id,
		})

		for _, child := range issue.RealChildren {
			addIssueToFG(child, issue.Id, group)
		}
	}

	addTrackerToFG = func(tracker *TrackerNode) {
		groupName := fmt.Sprintf("tracker_%d", tracker.TrackerId)
		nodes = append(nodes, FGNode{
			ID:    tracker.Id,
			Name:  EscapeDotString(fmt.Sprintf("[%s] %s", tracker.Id, tracker.Text)),
			Group: groupName,
			Color: "#2ECC40",
			Val:   4, // 트래커는 더 크게 표시
		})
		links = append(links, FGLink{
			Source: rootTracker.Id,
			Target: tracker.Id,
		})

		for _, issue := range tracker.Children {
			addIssueToFG(issue, tracker.Id, groupName)
		}
	}

	// 1. 루트 추가
	nodes = append(nodes, FGNode{
		ID:    rootTracker.Id,
		Name:  EscapeDotString(rootTracker.Text),
		Group: "root",
		Color: "#FF851B",
		Val:   8, // 루트는 가장 크게 표시
	})

	// 2. 하위 트래커 및 이슈 추가
	for _, childTracker := range vaildChildTracker {
		addTrackerToFG(childTracker)
	}

	// 3. 하이퍼링크 기반 참조 엣지 추가 (complexity 계산 로직에서 발견된 관계들)
	for fromID, toIDs := range linkRefs {
		for _, toID := range toIDs {
			links = append(links, FGLink{
				Source: fromID,
				Target: toID,
			})
		}
	}

	graphData := FGData{
		Nodes: nodes,
		Links: links,
	}

	graphDataJSON, _ := json.Marshal(graphData)

	// Force-Graph WebGL 렌더링을 위한 HTML 템플릿
	htmlTemplate := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Codebeamer Interactive Graph</title>
    <style type="text/css">
        body, html { font-family: sans-serif; height: 100%; margin: 0; padding: 0; overflow: hidden; background-color: #000011; }
        #graph { width: 100vw; height: 100vh; }
        .node-label {
            color: #fff;
            font-size: 12px;
            background: rgba(0, 0, 0, 0.7);
            padding: 4px 8px;
            border-radius: 4px;
            pointer-events: none;
        }
    </style>
    <!-- 로컬에 저장된 WebGL 기반 force-graph 라이브러리 참조 -->
    <script type="text/javascript" src="force-graph.min.js"></script>
</head>
<body>
<div id="graph"></div>
<script type="text/javascript">
    document.addEventListener('DOMContentLoaded', function(){
        const graphData = {{.GraphData}};

        const elem = document.getElementById('graph');

        // 1. 변수를 먼저 선언하여 스코프 이전에 평가되는 것을 방지
        let Graph;
        
        // 2. 인스턴스 초기화 할당
        Graph = ForceGraph()(elem)
            .graphData(graphData)
            .nodeId('id')
            .nodeColor('color')
            .nodeVal('val')
            .nodeLabel('name')
            .enableNodeDrag(false) // 사용자의 드래그를 비활성화하여 오직 조회 위주의 부드러운 패닝만 지원
            .linkColor(() => 'rgba(255,255,255,0.2)')
            .linkWidth(0.5)
            .cooldownTicks(100)
            .zoom(0.5);
            
        // 초기 물리엔진 파라미터 튜닝: 기본 노드들이 훨씬 오밀조밀하게 뭉치도록 설정
        // d3 객체가 글로벌에 없으므로 기존에 내장된 force 엔진 인스턴스를 직접 수정합니다.
        Graph.d3Force('charge').strength(-15);
        Graph.d3Force('link').distance(10);
        
        // 3. 줌 기반 클러스터링(Semantic Zoom) 및 시각적 LOD 구현
        
        let currentZoom = 0.5;
        let isClustered = false;
        const ZOOM_THRESHOLD = 1.0; // 너무 줌인해야 펼쳐지지 않도록 임계점 조정
        
        // 줌 이벤트 리스너: 특정 배율을 넘나들 때 데이터 교체 대신 시각적 가시성 변경
        Graph.onZoom(z => {
            currentZoom = z;
            
            // 데이터 클러스터링 스위치 로직
            if (currentZoom < ZOOM_THRESHOLD && !isClustered) {
                // 축소 모드: ISSUE 노드 및 연결된 자잘한 링크 시각적으로 숨기기
                isClustered = true;
                
                Graph.nodeVisibility(node => node.id === 'ROOT' || node.id.startsWith('TRACKER'));
                Graph.linkColor(link => {
                    let sId = link.source.id || link.source;
                    let tId = link.target.id || link.target;
                    // 줌 아웃 시 ROOT와 TRACKER 간의 링크만 보이게 처리
                    if ((sId === 'ROOT' && tId.startsWith('TRACKER')) || (tId === 'ROOT' && sId.startsWith('TRACKER'))) {
                        return 'rgba(255,255,255,0.2)';
                    }
                    return 'rgba(255,255,255,0)';
                });
                
                // 클러스터 덩어리들이 너무 멀어지지 않도록 응집도 단단하게 유지
                Graph.d3Force('link').distance(40); 
                Graph.d3Force('charge').strength(-40);
                Graph.zoomToFit(400); // 클러스터(트래커) 단위로 다시 화면에 정렬
                
            } else if (currentZoom >= ZOOM_THRESHOLD && isClustered) {
                // 확대 모드: 모든 이슈 노드와 링크 활성화
                isClustered = false;
                
                Graph.nodeVisibility(() => true);
                Graph.linkColor(() => 'rgba(255,255,255,0.2)');
                
                // 개별 노드 모드일 때는 거리를 가깝게 조정
                Graph.d3Force('link').distance(10);
                Graph.d3Force('charge').strength(-15);
            }
        });

        // 4. 인스턴스가 완전히 할당된 후 콜백 추가
        Graph.onEngineStop(() => {
            if (Graph && currentZoom === 0.5) { // 초기화 1회만 ToFit
                Graph.zoomToFit(400);
            }
        });
			
        // 클릭 시 해당 노드로 줌인 애니메이션
        Graph.onNodeClick(node => {
            Graph.centerAt(node.x, node.y, 1000);
            Graph.zoom(8, 2000);
        });
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
		"GraphData": string(graphDataJSON),
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

	// 매개변수로 브라우저 오픈이 전달되지 않으면 (ex. 테스트 환경 등) 생략
	if !openBrowser {
		Logger.Info("openBrowser flag is false, skipping browser launch")
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
