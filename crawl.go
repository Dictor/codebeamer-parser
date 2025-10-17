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
func FindRootTrackerByName(taskCtx context.Context, projectId string, targetTrackerName string) (*RootTrackerNode, error) {
	Logger.WithFields(logrus.Fields{
		"targetName": targetTrackerName,
		"projectId":  projectId,
	}).Debug("FillTrackerChild")

	// 호스트 URL에 접속하여, 내부 JS 런타임에서 fetch를 수행하여 POST API 호출
	trackerHomePageTreeResult := ""
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(CodebeamerHost),
		executeFetchInPage(
			fmt.Sprintf(GetTrackerHomePageTreeUrl, projectId),
			createFetchOption("POST", false, nil),
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
func FillTrackerChild(taskCtx context.Context, targetTracker *TrackerNode) error {
	Logger.WithFields(logrus.Fields{
		"trackerId": targetTracker.Id,
	}).Debug("FillTrackerChild")

	// 주어진 트래커 인스턴스의 페이지에 접속한 후, JS 런타임에서 config 변수 값을 평가
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(fmt.Sprintf(CodebeamerHost+TrackerPageUrl, targetTracker.Id)),
		waitUntilJSVariableIsDefined(TreeConfigDataExpression, 10*time.Second, 1*time.Second),
		chromedp.Evaluate(TreeConfigDataExpression, targetTracker),
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
func FillIssueChild(taskCtx context.Context, targetIssue *IssueNode, parentTrackerId string) error {
	Logger.WithFields(logrus.Fields{
		"issueId":   targetIssue.Id,
		"trackerId": parentTrackerId,
	}).Debug("FillIssueChild")

	// 해당 이슈에 자식이 없다면 검색할 필요도 없음
	if !targetIssue.HasChildren {
		return nil
	}

	// 주어진 코드비머 페이지에서 좌측 트리를 탐색하여 하위 노드를 모두 얻어옴
	childString := ""
	err := chromedp.Run(taskCtx,
		executeFetchInPage(
			TreeAjaxUrl,
			createFetchOption("POST", false, NewTrackerTreeRequest(parentTrackerId, targetIssue.Id, "")),
			&childString,
		),
	)
	if err != nil {
		return err
	}

	// 얻어온 결과를 파싱
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
func RecursiveFillIssueChild(taskCtx context.Context, issue *IssueNode, parentTrackerId string, sleepPerFill time.Duration) {
	if err := FillIssueChild(taskCtx, issue, parentTrackerId); err != nil {
		Logger.WithError(err).WithField("issueId", issue.Id).Warn("failed to process issue")
		return
	}
	if !issue.HasChildren {
		return
	}
	for _, child := range issue.RealChildren {
		time.Sleep(sleepPerFill)
		RecursiveFillIssueChild(taskCtx, child, parentTrackerId, sleepPerFill)
	}
}
