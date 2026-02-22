package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

// 주어진 이름과 프로젝트 ID으로 최상위 트래커를 찾습니다.
func FindRootTrackerByName(taskCtx context.Context, config ParsingConfig, targetTrackerName string) (*RootTrackerNode, error) {
	Logger.WithFields(logrus.Fields{
		"targetName": targetTrackerName,
		"projectId":  config.FcuProjectId,
	}).Debug("FillTrackerChild")

	// csrf 처리
	csrfValue, enableCsrf := taskCtx.Value("csrfToken").(string)
	opt := createFetchOption("POST", false, nil, enableCsrf, csrfValue)

	// 호스트 URL에 접속하여, 내부 JS 런타임에서 fetch를 수행하여 POST API 호출
	trackerHomePageTreeResult := ""
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(config.CodebeamerHost),
		executeFetchInPage(
			fmt.Sprintf(config.GetTrackerHomePageTreeUrl, config.FcuProjectId),
			opt,
			&trackerHomePageTreeResult,
		),
	)
	if err != nil {
		return nil, err
	}

	// API 결과 파싱
	treeNodes := []RootTrackerNode{}
	if err := json.Unmarshal([]byte(trackerHomePageTreeResult), &treeNodes); err != nil {
		return nil, err
	}

	// API 결과에서 주어진 이름을 기준으로 최상위 트래커 찾음
	tracker, trackerFound := lo.Find(treeNodes, func(item RootTrackerNode) bool {
		return item.Text == targetTrackerName
	})
	if !trackerFound {
		return nil, nil
	}

	return &tracker, nil
}

// 주어진 트래커 인스턴스의 자식 트래커 인스턴스를 찾음
func FillTrackerChild(taskCtx context.Context, config ParsingConfig, targetTracker *TrackerNode) error {
	Logger.WithFields(logrus.Fields{
		"trackerId": targetTracker.Id,
	}).Debug("FillTrackerChild")

	// 주어진 트래커 인스턴스의 페이지에 접속한 후, JS 런타임에서 config 변수 값을 평가
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(fmt.Sprintf(config.CodebeamerHost+config.TrackerPageUrl, targetTracker.Id)),
		waitUntilJSVariableIsDefined(config.TreeConfigDataExpression, time.Duration(config.JsVariableWaitTimeout)*time.Second, 1*time.Second),
		chromedp.Evaluate(config.TreeConfigDataExpression, targetTracker),
	)
	if err != nil {
		return err
	}

	// 자식의 존재 여부를 단언
	for _, issue := range targetTracker.Children {
		issue.AssertChild()
	}

	return nil
}

// 주어진 이슈 인스턴스의 자식 이슈 인스턴스를 찾음
func FillIssueChild(taskCtx context.Context, config ParsingConfig, targetIssue *IssueNode, parentTrackerId string) error {
	Logger.WithFields(logrus.Fields{
		"issueId":   targetIssue.Id,
		"trackerId": parentTrackerId,
	}).Debug("FillIssueChild")

	// 해당 이슈에 자식이 없다면 검색할 필요도 없음
	if !targetIssue.HasChildren {
		return nil
	}

	// csrf 처리
	csrfValue, enableCsrf := taskCtx.Value("csrfToken").(string)
	opt := createFetchOption("POST", false, NewTrackerTreeRequest(parentTrackerId, config.FcuProjectId, targetIssue.Id, ""), enableCsrf, csrfValue)

	// 주어진 코드비머 페이지에서 좌측 트리를 탐색하여 하위 노드를 모두 얻어옴
	childString := ""
	err := chromedp.Run(taskCtx,
		executeFetchInPage(
			config.TreeAjaxUrl,
			opt,
			&childString,
		),
	)
	if err != nil {
		return err
	}

	// 얻어온 결과를 파싱
	Logger.WithFields(logrus.Fields{
		"issueId":   targetIssue.Id,
		"trackerId": parentTrackerId,
		"result":    childString,
	}).Debug("FillIssueNewTrackerTreeRequest fetch result")
	if err := json.Unmarshal([]byte(childString), &targetIssue.RealChildren); err != nil {
		return err
	}

	// 얻어온 자식들이 자식을 가지는지 단언 (내 기준으로 손자 노드)
	for _, childIssue := range targetIssue.RealChildren {
		childIssue.AssertChild()
	}
	return nil
}

// 최상위 트래커 -> 자식 트래커 -> 자식 이슈 까지 3단계의 자료형을 완성했다면 1대 자식 이슈까지 얻어온 것임
// 이때 2대 이상의 하위 트래커를 재귀적으로 탐색하며 자료형을 완결시킴
func RecursiveFillIssueChild(taskCtx context.Context, config ParsingConfig, issue *IssueNode, parentTrackerId string, sleepPerFill time.Duration, weight float64, onProgress func(increment float64, node *IssueNode)) {
	if err := FillIssueChild(taskCtx, config, issue, parentTrackerId); err != nil {
		Logger.WithError(err).WithField("issueId", issue.Id).Warn("failed to process issue")
		if onProgress != nil {
			onProgress(weight, issue)
		}
		return
	}
	if !issue.HasChildren || len(issue.RealChildren) == 0 {
		if onProgress != nil {
			onProgress(weight, issue)
		}
		return
	}

	var chunk float64
	if onProgress != nil {
		chunk = weight / float64(len(issue.RealChildren)+1)
		onProgress(chunk, issue)
	}

	for _, child := range issue.RealChildren {
		time.Sleep(sleepPerFill)
		RecursiveFillIssueChild(taskCtx, config, child, parentTrackerId, sleepPerFill, chunk, onProgress)
	}
}

// 자식 트래커의 모든 하위 이슈의 Content를 얻어와 채움
func FillChildIssueContent(taskCtx context.Context, config ParsingConfig, targetTracker *TrackerNode, weight float64, onProgress func(increment float64, node *IssueNode)) {
	Logger.WithFields(logrus.Fields{
		"trackerId": targetTracker.Id,
	}).Debug("FillChildIssueContent")

	totalIssues := 0
	var countIssues func(issue *IssueNode) int
	countIssues = func(issue *IssueNode) int {
		c := 1
		for _, child := range issue.RealChildren {
			c += countIssues(child)
		}
		return c
	}
	for _, issue := range targetTracker.Children {
		totalIssues += countIssues(issue)
	}

	var increment float64
	if totalIssues > 0 {
		increment = weight / float64(totalIssues)
	}

	var recursiveFillIssueContent func(issue *IssueNode) error
	recursiveFillIssueContent = func(issue *IssueNode) error {
		Logger.WithFields(logrus.Fields{
			"trackerId": targetTracker.Id,
		}).Debug("  - recursiveFillIssueContent")

		taskCtxTimeout, taskCtxCancel := context.WithTimeout(taskCtx, time.Second*time.Duration(config.JsVariableWaitTimeout))
		var innerHTML []string
		err := chromedp.Run(taskCtxTimeout,
			chromedp.Navigate(fmt.Sprintf(config.CodebeamerHost+config.IssuePageUrl, issue.Id)),
			chromedp.WaitReady(config.IssueContentSelector, chromedp.ByQuery),
			getInnerHtmlBySelector(config.IssueContentSelector, &innerHTML),
		)
		taskCtxCancel()

		if err != nil {
			Logger.WithFields(logrus.Fields{
				"trackerId": targetTracker.Id,
				"issueId":   issue.Id,
			}).WithError(err).Error("failed to FillIssueContent: chromedp fail")
			issue.Content = ""
		} else {
			if len(innerHTML) < 1 {
				Logger.WithFields(logrus.Fields{
					"trackerId": targetTracker.Id,
					"issueId":   issue.Id,
				}).Error("failed to FillIssueContent: empty innerHTML")
			} else {
				issue.Content = innerHTML[0]
			}
		}

		if onProgress != nil {
			onProgress(increment, issue)
		}

		if issue.HasChildren {
			for _, childIssue := range issue.RealChildren {
				recursiveFillIssueContent(childIssue)
			}
		}

		return nil
	}

	for _, issue := range targetTracker.Children {
		if err := recursiveFillIssueContent(issue); err != nil {
			Logger.WithFields(logrus.Fields{
				"trackerId": targetTracker.Id,
				"issueId":   issue.Id,
			}).Error("recursiveFillIssueContent failed")
		}
	}
}

func GetCsrfToken(taskCtx context.Context, config ParsingConfig) (string, error) {
	// window.ajaxHeaders["X-CSRF-TOKEN"]
	Logger.WithFields(logrus.Fields{}).Debug("GetCsrfToken")

	var token string
	// 모든 페이지에 토큰 변수가 존재하므로, 현재 페이지에서 평가
	err := chromedp.Run(taskCtx,
		chromedp.Evaluate(config.CsrfTokenExpression, &token),
	)
	if err != nil {
		return "", err
	}

	if token == "" {
		return "", fmt.Errorf("cannot find token object")
	}

	return token, nil
}
