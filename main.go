package main

import (
	"context" // JSON 처리를 위해 추가
	"flag"
	"log" // os.Executable 사용 위해 추가
	"strconv"

	// 경로 조작 위해 추가
	"time"

	"github.com/chromedp/chromedp"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

var Logger *logrus.Logger = logrus.New()

func main() {
	debugLog := false
	flag.BoolVar(&debugLog, "debug", false, "print debug log")
	flag.Parse()

	if debugLog {
		Logger.SetLevel(logrus.DebugLevel)
	}
	Logger.SetFormatter(&logrus.TextFormatter{})

	Logger.Info("init chrome connection")
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), ChromeDevtoolsURL)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancelTask()

	Logger.Info("browser will be navigated to codebeamer page, please login until 10 sec")
	lo.Must0(
		chromedp.Run(taskCtx,
			chromedp.Navigate(CodebeamerHost),
			chromedp.Sleep(10*time.Second),
		),
	)

	Logger.Info("start find tracker")
	rootTracker := lo.Must1(FindRootTrackerByName(taskCtx, FcuProjectId, FcuRequirementName))
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

	Logger.Info("start find issue")
	for _, childTracker := range vaildChildTracker {
		for _, childIssue := range childTracker.Children {
			time.Sleep(300 * time.Millisecond)
			RecursiveFillIssueChild(taskCtx, childIssue, strconv.Itoa(childTracker.TrackerId), 300*time.Millisecond)
		}
	}
	Logger.Info("complete to issue")

	return
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
