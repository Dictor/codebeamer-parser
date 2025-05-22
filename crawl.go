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

func FindRootTrackerByName(taskCtx context.Context, projectId string, targetTrackerName string) (*RootTrackerNode, error) {
	Logger.WithFields(logrus.Fields{
		"targetName": targetTrackerName,
		"projectId":  projectId,
	}).Debug("FillTrackerChild")

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

	treeNodes := []RootTrackerNode{}
	if err := json.Unmarshal([]byte(trackerHomePageTreeResult), &treeNodes); err != nil {
		return nil, err
	}

	tracker, trackerFound := lo.Find(treeNodes, func(item RootTrackerNode) bool {
		return item.Text == targetTrackerName
	})
	if !trackerFound {
		return nil, nil
	}

	return &tracker, nil
}

func FillTrackerChild(taskCtx context.Context, targetTracker *TrackerNode) error {
	Logger.WithFields(logrus.Fields{
		"trackerId": targetTracker.Id,
	}).Debug("FillTrackerChild")

	err := chromedp.Run(taskCtx,
		chromedp.Navigate(fmt.Sprintf(CodebeamerHost+TrackerPageUrl, targetTracker.Id)),
		waitUntilJSVariableIsDefined(TreeConfigDataExpression, 10*time.Second, 1*time.Second),
		chromedp.Evaluate(TreeConfigDataExpression, targetTracker),
	)
	if err != nil {
		return err
	}

	for _, issue := range targetTracker.Children {
		issue.AssertChild()
	}

	return nil
}

func FillIssueChild(taskCtx context.Context, targetIssue *IssueNode, parentTrackerId string) error {
	Logger.WithFields(logrus.Fields{
		"issueId":   targetIssue.Id,
		"trackerId": parentTrackerId,
	}).Debug("FillIssueChild")

	if !targetIssue.HasChildren {
		return nil
	}

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

	if err := json.Unmarshal([]byte(childString), &targetIssue.RealChildren); err != nil {
		return err
	}

	for _, childIssue := range targetIssue.RealChildren {
		childIssue.AssertChild()
	}
	return nil
}

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
