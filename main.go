package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
	var debugLog, saveGraph, skipCrawling, guiMode bool
	var partialCrawling string
	flag.BoolVar(&debugLog, "debug", false, "print debug log")
	flag.BoolVar(&saveGraph, "graph", false, "save graph image")
	flag.BoolVar(&skipCrawling, "skip-crawl", false, "skip crawling, using result.json instead")
	flag.StringVar(&partialCrawling, "partial-crawl", "", "crawing only a tracker of given id")
	flag.BoolVar(&guiMode, "gui", false, "run in GUI mode")
	flag.Parse()

	if guiMode {
		startGUI(debugLog, saveGraph, skipCrawling, partialCrawling)
	} else {
		runLogic(debugLog, saveGraph, skipCrawling, partialCrawling)
	}
}

func runLogic(debugLog, saveGraph, skipCrawling bool, partialCrawling string) {

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
	v.SetDefault("issue_page_url", "/cb/issue/%s")
	v.SetDefault("tree_ajax_url", "/cb/trackers/ajax/tree.spr")
	v.SetDefault("tree_config_data_expression", "tree.config.data")
	v.SetDefault("interval_per_request_ms", 300)
	v.SetDefault("js_variable_wait_timeout_s", 10)
	v.SetDefault("issue_content_selector", ".wikiContent")
	v.SetDefault("csrf_token_expression", "window.ajaxHeaders['X-CSRF-TOKEN']")
	v.SetDefault("enable_csrf_token", true)
	v.SetDefault("enable_requirement_node_name_filtering", true)

	// 설정 파일 읽기
	Logger.Info("read setting file")
	lo.Must0(v.ReadInConfig())
	config := ParsingConfig{}
	lo.Must0(v.Unmarshal(&config))
	Logger.WithField("config", config).Debug("config")

	// 설정 값 검증
	Logger.Info("validate configuration")
	validate := validator.New()
	lo.Must0(validate.Struct(&config))

	// 사양 그래프 시각화를 위한 graphviz 초기화
	Logger.Info("initialize graphviz")
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
		vaildChildTracker, rootTracker = CrawlCodebeamer(taskCtx, config, time.Duration(config.IntervalPerRequest)*time.Millisecond, partialCrawling != "", partialCrawling)

		// 크롤링 결과를 저장
		Logger.Info("save crawled tracker info to files")
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
	IdToNode := map[string]*cgraph.Node{}

	// 루트 노드 생성
	gRootTracker := lo.Must(graph.CreateNodeByName(EscapeDotString(rootTracker.Id)))
	IdToNode[rootTracker.Id] = gRootTracker

	// 바로 하위의 최상위 트래커 노드 성
	for _, childTracker := range vaildChildTracker {
		gChildTracker := lo.Must(graph.CreateNodeByName(EscapeDotString(childTracker.Id)))
		graph.CreateEdgeByName("", gRootTracker, gChildTracker)
		childTracker.GraphNode = gChildTracker
		IdToNode[childTracker.Id] = gChildTracker
	}

	// 두번째로, 트래커의 하위 이슈를 모두 순회하며 그래프 생성
	Logger.Info("construct graph for tracker's child issues")
	var recursiveIssueGraph func(*IssueNode) *cgraph.Node
	recursiveIssueGraph = func(issue *IssueNode) *cgraph.Node {
		gIssue := lo.Must(graph.CreateNodeByName(EscapeDotString(issue.Id)))
		IdToNode[issue.Id] = gIssue
		for _, childIssue := range issue.RealChildren {
			gChildIssue := lo.Must(graph.CreateNodeByName(EscapeDotString(childIssue.Id)))
			IdToNode[childIssue.Id] = gChildIssue
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

	// 사양 복잡도를 계산하기 위해 이슈에서 실제 사양 텍스트를 추출
	Logger.Info("extract specification text from issues")
	var recursiveIssueText func(*IssueNode) []*IssueNode
	recursiveIssueText = func(issue *IssueNode) []*IssueNode {
		ret := []*IssueNode{}
		if config.EnableRequirementNodeNameFiltering {
			if issue.Text == config.RequirementNodeName {
				Logger.WithField("issueText", issue.Text).Debug("issue text matched with RequirementNodeName")
				ret = append(ret, issue)
			}
		} else {
			ret = append(ret, issue)
		}
		for _, childIssue := range issue.RealChildren {
			ret = append(ret, recursiveIssueText(childIssue)...)
		}
		return ret
	}

	// 사양 텍스트들에서 사양 복잡도를 계산
	Logger.Info("calculate specification complexity")
	complexity := map[string]int{}
	issueRegex := regexp.MustCompile(`ISSUE:(\d+)`)
	for _, childTracker := range vaildChildTracker {
		for _, childIssue := range childTracker.Children {
			issueNodes := recursiveIssueText(childIssue)
			complexity[EscapeDotString(childIssue.Title)] = lo.Reduce[*IssueNode, int](
				issueNodes,
				func(agg int, item *IssueNode, index int) int {
					Logger.WithField("itemTitle", item.Title).Debug("calculating complexity for item")
					for _, ci := range item.RealChildren {
						for fieldName, fieldVal := range map[string]string{"Text": ci.Text, "Content": ci.Content} {
							matches := issueRegex.FindAllStringSubmatch(fieldVal, -1)
							for _, m := range matches {
								issueId := m[1]
								Logger.WithFields(logrus.Fields{
									"issueId": issueId,
								}).Debugf("hyperlinked issue id matched in %s", fieldName)

								edgeFrom, fromOk := IdToNode[item.Id]
								edgeTo, toOk := IdToNode[issueId]
								if toOk && fromOk {
									Logger.WithFields(logrus.Fields{
										"fromId": item.Id,
										"toId":   issueId,
									}).Debug("edge from hyperlink")
									lo.Must1(graph.CreateEdgeByName("", edgeFrom, edgeTo))
								} else {
									Logger.Error("issue edge creation failed")
								}
							}
							agg += len(matches)
						}
					}
					return agg
				},
				0,
			)
		}
	}

	// 사양 그래프를 시각화
	if saveGraph {
		Logger.Info("render and save graph.svg")
		ctx := context.Background()
		file := lo.Must(os.OpenFile("graph.svg", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666))
		defer file.Close()
		lo.Must0(g.Render(ctx, graph, graphviz.SVG, file))
	}
	Logger.Info("complete to construct graph")

	// 사양 복잡도 결과를 파일로 저장
	Logger.Info("save calculated complexity to file")
	complexityJson := lo.Must(json.MarshalIndent(complexity, "", "  "))
	lo.Must0(os.WriteFile("complexity.json", complexityJson, 0666))
}

// 크롬 브라우저를 제어하여 코드 비머의 정보를 파싱
func CrawlCodebeamer(taskCtx context.Context, config ParsingConfig, delayPerRequest time.Duration, partialMode bool, partialId string) (vaildChildTracker []*TrackerNode, rootTracker *RootTrackerNode) {
	// 코드 비머 Host URL로 접속하여 10초동안 대기
	// 이 10초가 만료되기 전에 사용자는 크롬 브라우저 적절한 자격 증명으로 로그인을 완료해야함
	Logger.Info("browser will be navigated to codebeamer page, please login until 10 sec")
	lo.Must0(
		chromedp.Run(taskCtx,
			chromedp.Navigate(config.CodebeamerHost),
			chromedp.Sleep(10*time.Second),
		),
	)

	// API 중 CSRF 없으면 403 반환하는 API를 위한 호환성용
	if config.EnableCsrfToken {
		Logger.Info("fetch CSRF token for API compatibility")
		csrfToken := lo.Must1(GetCsrfToken(taskCtx, config))
		taskCtx = context.WithValue(taskCtx, "csrfToken", csrfToken)
		Logger.WithField("value", csrfToken).Debug("csrf token updated")
	}

	// 최상위 트래커를 검색
	Logger.Info("start to find tracker")
	rootTracker = lo.Must1(FindRootTrackerByName(taskCtx, config, config.FcuRequirementName))

	if partialMode {
		Logger.WithField("target_id", partialId).Info("partial tracker find mode enabled")
	}

	// 전체 진행률은 트래커 스캔에 30%, 이슈 스캔에 70% 비중을 둡니다
	// 전체 진행률은 트래커 스캔에 30%, 이슈 스캔에 70% 비중을 둡니다
	const trackerProgressRatio = 30.0
	const issueProgressRatio = 70.0

	// 최상위 트래커의 하위 트래커 목록을 재귀적으로 탐색
	Logger.WithField("stepName", "(2/5) filling root and child trackers").Info("find child trackers of root tracker")
	vaildChildTracker = []*TrackerNode{}
	rootChildrenCount := len(rootTracker.Children)
	trackerStartTime := time.Now()
	for i, childTracker := range rootTracker.Children {
		// 부분 파싱을 위한 테스트
		childId := childTracker.Id
		childTrackerId := strconv.Itoa(childTracker.TrackerId)
		if partialMode && (childId == partialId || childTrackerId == partialId) {
			Logger.WithFields(logrus.Fields{
				"child_id":         childId,
				"child_tracker_id": childTrackerId,
			}).Debug("child tracker passed because id doesn't matched")
			continue
		} else {
			Logger.WithFields(logrus.Fields{
				"child_id":         childId,
				"child_tracker_id": childTrackerId,
			}).Debug("child matched for partial crawling")
		}

		time.Sleep(delayPerRequest)
		progress := (float64(i+1) / float64(rootChildrenCount)) * trackerProgressRatio

		elapsed := time.Since(trackerStartTime)
		eta := time.Duration(0)
		if progress > 0 && progress < 100 {
			eta = time.Duration(float64(elapsed) * (100.0 - progress) / progress)
		}

		Logger.WithFields(logrus.Fields{
			"trackerId": childTracker.Id,
			"progress":  fmt.Sprintf("%.2f%%", progress),
			"eta":       eta.Round(time.Second).String(),
			"step":      fmt.Sprintf("%d/%d", i+1, rootChildrenCount),
			"stepName":  "(3/5) filling tracker's children",
		}).Info("fill tracker child")
		if err := FillTrackerChild(taskCtx, config, childTracker); err == nil {
			vaildChildTracker = append(vaildChildTracker, childTracker)
		} else {
			Logger.WithError(err).WithField("trackerId", childTracker.TrackerId).Warn("failed to process tracker")
		}
	}
	Logger.WithField("count", len(vaildChildTracker)).Info("complete to find tracker")

	// 찾은 모든 트래커들의 이슈를 탐색
	Logger.Info("start to find issue")
	validTrackerCount := len(vaildChildTracker)
	issueStartTime := time.Now()
	var totalProgress float64 = trackerProgressRatio

	for i, childTracker := range vaildChildTracker {
		trackerWeight := issueProgressRatio / float64(validTrackerCount)
		Logger.WithFields(logrus.Fields{
			"trackerId": childTracker.Id,
			"step":      fmt.Sprintf("%d/%d", i+1, validTrackerCount),
			"stepName":  "(4/5) filling issue's children recursively",
		}).Info("find issues for tracker")

		childIssueCount := len(childTracker.Children)
		if childIssueCount == 0 {
			// 빈 트래커라도 탐색과정을 진행한 것으로 간주하여 전체 진행도를 정상적으로 올리기 위해 trackerWeight를 추가
			totalProgress += trackerWeight
		} else {
			findWeight := trackerWeight * 0.5
			fillWeight := trackerWeight * 0.5

			for j, childIssue := range childTracker.Children {
				time.Sleep(delayPerRequest)
				issueWeight := findWeight / float64(childIssueCount)

				RecursiveFillIssueChild(taskCtx, config, childIssue, strconv.Itoa(childTracker.TrackerId), 300*time.Millisecond, issueWeight, func(inc float64, node *IssueNode) {
					totalProgress += inc

					elapsed := time.Since(issueStartTime)
					eta := time.Duration(0)
					if totalProgress > 0 && totalProgress < 100 {
						eta = time.Duration(float64(elapsed) * (100.0 - totalProgress) / totalProgress)
					}

					Logger.WithFields(logrus.Fields{
						"issueId":  node.Id,
						"progress": fmt.Sprintf("%.2f%%", totalProgress),
						"eta":      eta.Round(time.Second).String(),
						"step":     fmt.Sprintf("tracker=%d/%d top-issue=%d/%d", i+1, validTrackerCount, j+1, childIssueCount),
						"stepName": "(4/5) filling issue's children recursively",
					}).Info("fill issue child (recursive)")
				})
			}

			// 찾은 이슈의 본문 탐색 탐색
			Logger.WithFields(logrus.Fields{
				"trackerId": childTracker.Id,
				"stepName":  "(5/5) filling issue's content",
			}).Info("fill issue content for tracker")
			FillChildIssueContent(taskCtx, config, childTracker, fillWeight, func(inc float64, node *IssueNode) {
				totalProgress += inc

				elapsed := time.Since(issueStartTime)
				eta := time.Duration(0)
				if totalProgress > 0 && totalProgress < 100 {
					eta = time.Duration(float64(elapsed) * (100.0 - totalProgress) / totalProgress)
				}

				Logger.WithFields(logrus.Fields{
					"issueId":  node.Id,
					"progress": fmt.Sprintf("%.2f%%", totalProgress),
					"eta":      eta.Round(time.Second).String(),
					"step":     fmt.Sprintf("tracker=%d/%d content-fill", i+1, validTrackerCount),
					"stepName": "(5/5) filling issue's content",
				}).Info("fill issue content")
			})
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
