package main

import (
	"time"

	"github.com/sirupsen/logrus"
)

// RecursiveFillIssueChild recursively fills child issues using the provided Crawler.
func RecursiveFillIssueChild(crawler Crawler, issue *IssueNode, parentTrackerId string, sleepPerFill time.Duration, weight float64, onProgress func(increment float64, node *IssueNode)) {
	if err := crawler.FillIssueChild(issue, parentTrackerId); err != nil {
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
		RecursiveFillIssueChild(crawler, child, parentTrackerId, sleepPerFill, chunk, onProgress)
	}
}

// FillChildIssueContent fills the content of all child issues in a tracker using the provided Crawler.
func FillChildIssueContent(crawler Crawler, targetTracker *TrackerNode, weight float64, onProgress func(increment float64, node *IssueNode)) {
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
			"issueId":   issue.Id,
		}).Debug("  - recursiveFillIssueContent")

		err := crawler.FillIssueContent(issue)
		if err != nil {
			Logger.WithFields(logrus.Fields{
				"trackerId": targetTracker.Id,
				"issueId":   issue.Id,
			}).WithError(err).Error("failed to FillIssueContent")
			issue.Content = ""
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
