package main

import (
	"context"
	"encoding/json"
	"flag"
	"html"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"time"

	"github.com/chromedp/chromedp"
	"github.com/go-playground/validator/v10"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
)

var Logger *logrus.Logger = logrus.New()

// 프로그램의 진입점
// 사용자의 입력을 파싱하고 전체 로직을 수행합니다.
func main() {
	// 사용자의 입력을 flag로 받아옴
	var debugLog, saveGraph, skipCrawling bool
	flag.BoolVar(&debugLog, "debug", false, "print debug log")
	flag.BoolVar(&saveGraph, "graph", false, "save graph image")
	flag.BoolVar(&skipCrawling, "skip-crawl", false, "skip crawling, using result.json instead")
	flag.Parse()

	// debug 플래그가 활성화된 경우, 로거를 디버그 모드로 변경
	if debugLog {
		Logger.SetLevel(logrus.DebugLevel)
	}
	Logger.SetFormatter(&logrus.TextFormatter{})

	// 설정 초기화
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	// 설정 기본값 설정
	// 아래 값은 특정 회사나 프로젝트, 용도에 귀속되지 않고 코드비머 체계 자체에서 범용적으로 사용되므로 기본 값으로 설정함
	v.SetDefault("chrome_devtools_url", "ws://127.0.0.1:9222/devtools/browser")
	v.SetDefault("get_tracker_home_page_tree_url", "/cb/ajax/getTrackerHomePageTree.spr?proj_id=%s")
	v.SetDefault("tracker_page_url", "/cb/tracker/%s")
	v.SetDefault("tree_ajax_url", "/cb/trackers/ajax/tree.spr")
	v.SetDefault("tree_config_data_expression", "tree.config.data")
	v.SetDefault("interval_per_request_ms", 300)
	v.SetDefault("js_variable_wait_timeout_s", 10)

	// 설정 파일 읽기
	Logger.Info("read setting file")
	lo.Must0(v.ReadInConfig())
	config := ParsingConfig{}
	lo.Must0(v.Unmarshal(&config))
	Logger.WithField("config", config).Debug("config")

	// 설정 값 검증
	validate := validator.New()
	lo.Must0(validate.Struct(&config))

	// 사양 그래프 시각화를 위한 graphviz 초기화
	g := lo.Must(graphviz.New(context.Background()))
	defer g.Close()
	graph := lo.Must(g.Graph())
	defer graph.Close()

	// 이전에 크롤링 결과가 저장되어있는지 확인하고, 존재하면 재사용
	var rootTracker *RootTrackerNode
	var vaildChildTracker []*TrackerNode
	if skipCrawling {
		// 존재하므로, 크롤링을 스킵하고 재사용
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
		// 존재하지 않으므로, 크롤링 진행
		// 크롬 브라우저 초기화
		Logger.Info("init chrome connection")
		allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), config.ChromeDevtoolsURL)
		defer cancelAlloc()
		taskCtx, cancelTask := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
		defer cancelTask()

		// 크롤링 진행
		// 이때 요청 당 간격을 300ms으로 설정하여 의도치 않은 DoS 공격을 방지
		vaildChildTracker, rootTracker = CrawlCodebeamer(taskCtx, config, time.Duration(config.IntervalPerRequest)*time.Millisecond)

		// 크롤링 결과를 저장
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

	// 사양 그래프를 생성
	// 첫번째로, 모든 트래커를 재귀적으로 순회하며 그래프 생성
	Logger.Info("start to construct graph")
	gRootTracker := lo.Must(graph.CreateNodeByName(EscapeDotString(rootTracker.Id)))
	for _, childTracker := range vaildChildTracker {
		gChildTracker := lo.Must(graph.CreateNodeByName(EscapeDotString(childTracker.Id)))
		graph.CreateEdgeByName("", gRootTracker, gChildTracker)
		childTracker.GraphNode = gChildTracker
	}
	// 두번째로, 트래커의 하위 이슈를 모두 순회하며 그래프 생성
	var recursiveIssueGraph func(*IssueNode) *cgraph.Node
	recursiveIssueGraph = func(issue *IssueNode) *cgraph.Node {
		gIssue := lo.Must(graph.CreateNodeByName(EscapeDotString(issue.Id)))
		for _, childIssue := range issue.RealChildren {
			gChildIssue := lo.Must(graph.CreateNodeByName(EscapeDotString(childIssue.Id)))
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

	// 사양 그래프를 시각화
	if saveGraph {
		ctx := context.Background()
		file := lo.Must(os.OpenFile("graph.svg", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666))
		defer file.Close()
		lo.Must0(g.Render(ctx, graph, graphviz.SVG, file))
	}
	Logger.Info("complete to construct graph")

	// 사양 복잡도를 계산하기 위해 이슈에서 실제 사양 텍스트를 추출
	var recursiveIssueText func(*IssueNode) []*IssueNode
	recursiveIssueText = func(issue *IssueNode) []*IssueNode {
		ret := []*IssueNode{}
		if issue.Text == config.RequirementNodeName {
			ret = append(ret, issue)
		}
		for _, childIssue := range issue.RealChildren {
			ret = append(ret, recursiveIssueText(childIssue)...)
		}
		return ret
	}
	// 사양 텍스트들에서 사양 복잡도를 계산
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

	// 사양 복잡도 결과를 파일로 저장
	complexityJson := lo.Must(json.MarshalIndent(complexity, "", "  "))
	lo.Must0(os.WriteFile("complexity.json", complexityJson, 0666))
}

// 크롬 브라우저를 제어하여 코드 비머의 정보를 파싱
func CrawlCodebeamer(taskCtx context.Context, config ParsingConfig, delayPerRequest time.Duration) (vaildChildTracker []*TrackerNode, rootTracker *RootTrackerNode) {
	// 코드 비머 Host URL로 접속하여 10초동안 대기
	// 이 10초가 만료되기 전에 사용자는 크롬 브라우저 적절한 자격 증명으로 로그인을 완료해야함
	Logger.Info("browser will be navigated to codebeamer page, please login until 10 sec")
	lo.Must0(
		chromedp.Run(taskCtx,
			chromedp.Navigate(config.CodebeamerHost),
			chromedp.Sleep(10*time.Second),
		),
	)

	// 최상위 트래커를 검색
	Logger.Info("start to find tracker")
	rootTracker = lo.Must1(FindRootTrackerByName(taskCtx, config, config.FcuRequirementName))

	// 최상위 트래커의 하위 트래커 목록을 재귀적으로 탐색
	vaildChildTracker = []*TrackerNode{}
	for _, childTracker := range rootTracker.Children {
		time.Sleep(delayPerRequest)
		if err := FillTrackerChild(taskCtx, config, childTracker); err == nil {
			vaildChildTracker = append(vaildChildTracker, childTracker)
		} else {
			Logger.WithError(err).WithField("trackerId", childTracker.TrackerId).Warn("failed to process tracker")
		}
	}
	Logger.WithField("count", len(vaildChildTracker)).Info("complete to find tracker")

	// 찾은 모든 트래커들의 이슈를 탐색
	Logger.Info("start to find issue")
	for _, childTracker := range vaildChildTracker {
		for _, childIssue := range childTracker.Children {
			time.Sleep(delayPerRequest)
			RecursiveFillIssueChild(taskCtx, config, childIssue, strconv.Itoa(childTracker.TrackerId), 300*time.Millisecond)
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
