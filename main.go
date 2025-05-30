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
	var debugLog, saveGraph, skipCrawling bool
	flag.BoolVar(&debugLog, "debug", false, "print debug log")
	flag.BoolVar(&saveGraph, "graph", false, "save graph image")
	flag.BoolVar(&skipCrawling, "skip-crawl", false, "skip crawling, using result.json instead")
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

	// make up codebeamer info
	var rootTracker *RootTrackerNode
	var vaildChildTracker []*TrackerNode
	if skipCrawling {
		// restore info
		Logger.Info("restore saved info")
		lo.Must0(
			json.Unmarshal(
				lo.Must(os.ReadFile("valid_child_tracker.json")),
				&vaildChildTracker,
			),
		)
		lo.Must0(
			json.Unmarshal(
				lo.Must(os.ReadFile("root_tracker.json")),
				&rootTracker,
			),
		)
	} else {
		// init chrome
		Logger.Info("init chrome connection")
		allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), ChromeDevtoolsURL)
		defer cancelAlloc()
		taskCtx, cancelTask := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
		defer cancelTask()

		// crawling
		vaildChildTracker, rootTracker = CrawlCodebeamer(taskCtx, 300*time.Millisecond)

		// save parse result
		lo.Must0(
			os.WriteFile(
				"valid_child_tracker.json",
				lo.Must(json.MarshalIndent(vaildChildTracker, "", "  ")),
				0666,
			),
		)
		lo.Must0(
			os.WriteFile(
				"root_tracker.json",
				lo.Must(json.MarshalIndent(rootTracker, "", "  ")),
				0666,
			),
		)
	}

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

	// calculate complexity
	var recursiveIssueText func(*IssueNode) []*IssueNode
	recursiveIssueText = func(issue *IssueNode) []*IssueNode {
		ret := []*IssueNode{}
		if issue.Text == RequirementNodeName {
			ret = append(ret, issue)
		}
		for _, childIssue := range issue.RealChildren {
			ret = append(ret, recursiveIssueText(childIssue)...)
		}
		return ret
	}

	complexity := map[string]int{}
	for _, childTracker := range vaildChildTracker {
		for _, childIssue := range childTracker.Children {
			complexity[EscapeDotString(childIssue.Title)] = lo.Reduce[*IssueNode, int](
				recursiveIssueText(childIssue),
				func(agg int, item *IssueNode, index int) int {
					for _, ci := range item.RealChildren {
						agg += strings.Count(ci.Text, "ISSUE:")
					}
					return agg
				},
				0,
			)
		}
	}

	// save complexity result
	complexityJson := lo.Must(json.MarshalIndent(complexity, "", "  "))
	lo.Must0(os.WriteFile("complexity.json", complexityJson, 0666))
}

func CrawlCodebeamer(taskCtx context.Context, delayPerRequest time.Duration) (vaildChildTracker []*TrackerNode, rootTracker *RootTrackerNode) {
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
	rootTracker = lo.Must1(FindRootTrackerByName(taskCtx, FcuProjectId, FcuRequirementName))

	// check sub-root tracker
	vaildChildTracker = []*TrackerNode{}
	for _, childTracker := range rootTracker.Children {
		time.Sleep(delayPerRequest)
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
			time.Sleep(delayPerRequest)
			RecursiveFillIssueChild(taskCtx, childIssue, strconv.Itoa(childTracker.TrackerId), 300*time.Millisecond)
		}
	}
	Logger.Info("complete to find issue")
	return
}

func EscapeDotString(s string) string {
	var cleanHTMLRegex = regexp.MustCompile("<[^>]*>")
	processedString := html.UnescapeString(s)
	processedString = cleanHTMLRegex.ReplaceAllString(processedString, "")
	processedString = strings.TrimSpace(processedString)
	return processedString
}
