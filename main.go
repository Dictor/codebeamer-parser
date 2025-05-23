package main

import (
	"context" // JSON 처리를 위해 추가
	"encoding/json"
	"flag"
	"html"
	"log" // os.Executable 사용 위해 추가
	"os"
	"regexp"
	"strconv"
	"strings"

	// 경로 조작 위해 추가
	"time"

	"github.com/chromedp/chromedp"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
)

var Logger *logrus.Logger = logrus.New()

func main() {
	// flag
	var debugLog, saveGraph bool
	flag.BoolVar(&debugLog, "debug", false, "print debug log")
	flag.BoolVar(&saveGraph, "graph", false, "save graph image")
	flag.Parse()

	// logger setting
	if debugLog {
		Logger.SetLevel(logrus.DebugLevel)
	}
	Logger.SetFormatter(&logrus.TextFormatter{})

	// graphviz setting
	g := lo.Must(graphviz.New(context.Background()))
	defer g.Close()
	graph := lo.Must(g.Graph())
	defer graph.Close()

	// init chrome
	Logger.Info("init chrome connection")
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), ChromeDevtoolsURL)
	defer cancelAlloc()
	taskCtx, cancelTask := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancelTask()

	// navigate chrome for login
	Logger.Info("browser will be navigated to codebeamer page, please login until 10 sec")
	lo.Must0(
		chromedp.Run(taskCtx,
			chromedp.Navigate(CodebeamerHost),
			chromedp.Sleep(10*time.Second),
		),
	)

	// find root tracker
	Logger.Info("start to find tracker")
	rootTracker := lo.Must1(FindRootTrackerByName(taskCtx, FcuProjectId, FcuRequirementName))

	// check sub-root tracker
	vaildChildTracker := []*TrackerNode{}
	for _, childTracker := range rootTracker.Children {
		time.Sleep(300 * time.Millisecond)
		if err := FillTrackerChild(taskCtx, childTracker); err == nil {
			vaildChildTracker = append(vaildChildTracker, childTracker)
		} else {
			Logger.WithError(err).WithField("trackerId", childTracker.TrackerId).Warn("failed to process tracker")
		}
	}
	Logger.WithField("count", len(vaildChildTracker)).Info("complete to find tracker")

	// check issue
	Logger.Info("start to find issue")
	for _, childTracker := range vaildChildTracker {
		for _, childIssue := range childTracker.Children {
			time.Sleep(300 * time.Millisecond)
			RecursiveFillIssueChild(taskCtx, childIssue, strconv.Itoa(childTracker.TrackerId), 300*time.Millisecond)
		}
	}
	Logger.Info("complete to find issue")

	// construct graph
	Logger.Info("start to construct graph")
	gRootTracker := lo.Must(graph.CreateNodeByName(EscapeDotString(rootTracker.Text)))
	for _, childTracker := range vaildChildTracker {
		gChildTracker := lo.Must(graph.CreateNodeByName(EscapeDotString(childTracker.Text)))
		graph.CreateEdgeByName("", gRootTracker, gChildTracker)
		childTracker.GraphNode = gChildTracker
	}

	var recursiveIssueGraph func(*IssueNode) *cgraph.Node
	recursiveIssueGraph = func(issue *IssueNode) *cgraph.Node {
		gIssue := lo.Must(graph.CreateNodeByName(EscapeDotString(issue.Text)))
		for _, childIssue := range issue.RealChildren {
			gChildIssue := lo.Must(graph.CreateNodeByName(EscapeDotString(childIssue.Text)))
			graph.CreateEdgeByName("", gIssue, gChildIssue)
			recursiveIssueGraph(childIssue)
		}
		return gIssue
	}

	for _, childTracker := range vaildChildTracker {
		for _, childIssue := range childTracker.Children {
			recursiveIssueGraph(childIssue)
			graph.CreateEdgeByName("", childTracker.GraphNode.(*cgraph.Node), recursiveIssueGraph(childIssue))
		}
	}

	// save graph to image
	if saveGraph {
		ctx := context.Background()
		file := lo.Must(os.OpenFile("graph.svg", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666))
		defer file.Close()
		lo.Must0(g.Render(ctx, graph, graphviz.SVG, file))
	}
	Logger.Info("complete to construct graph")

	// save parse result
	resultJson := lo.Must(json.MarshalIndent(rootTracker, "", "  "))
	lo.Must0(os.WriteFile("result.json", resultJson, 0666))
	/*


			// < Step 4. 각 상세 사양 문서의 복잡도 계산 >
			for _, detailRq := range findResult {
				complexity := getComplexityOfDetailRQ(taskCtx, trackerId, detailRq)
				log.Printf("[Step 4] 상세 사양 문서 ID=%d의 복잡도=%d", detailRq.NodeId, complexity)
				totalLogResult = append(totalLogResult, logResult{
					DetailRqId: detailRq.Id,
					DetailRq:   detailRq,
					Complexity: complexity,
				})
			}
		}
	*/
}

/*
func getComplexityOfDetailRQ(chromeCtx context.Context, trackerId int, detailRQ *TreeType2Child) int {
	log.Printf("[getComplexityOfDetailRQ] 상세 사양 탐색 시작, 문서 ID=%s", detailRQ.Id)

	drqContent := ""
	err := chromedp.Run(chromeCtx,
		chromedp.Sleep(300*time.Millisecond),
		executeFetchInPage(
			TreeAjaxUrl,
			createFetchOption("POST", false, NewTrackerTreeRequest(trackerId, detailRQ.NodeId, "")),
			&drqContent,
		),
	)
	if err != nil {
		log.Printf("[getComplexityOfDetailRQ] 크롬 오류: %v", err)
		return 0
	}
	drqElements := []TreeType3Child{}
	if err := json.Unmarshal([]byte(drqContent), &drqElements); err != nil {
		log.Printf("[getComplexityOfDetailRQ] 파싱 오류: %v", err)
		return 0
	}

	ret := 0
	for _, element := range drqElements {
		if element.ListAttr.IconBgColor == "#ababab" {
			continue
		}
		ret += strings.Count(element.Text, "[ISSUE:")
	}

	return ret
}
*/

func EscapeDotString(s string) string {
	var cleanHTMLRegex = regexp.MustCompile("<[^>]*>")
	processedString := html.UnescapeString(s)
	processedString = cleanHTMLRegex.ReplaceAllString(processedString, "")
	processedString = strings.TrimSpace(processedString)
	return processedString
}
