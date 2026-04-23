package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

type ChromedpCrawler struct {
	config    ParsingConfig
	ctx       context.Context
	cancel    context.CancelFunc
	csrfToken string
}

func NewChromedpCrawler(config ParsingConfig) *ChromedpCrawler {
	return &ChromedpCrawler{
		config: config,
	}
}

func (c *ChromedpCrawler) Login() error {
	Logger.Info("init chrome connection")
	allocCtx, _ := chromedp.NewRemoteAllocator(context.Background(), c.config.ChromeDevtoolsURL)
	// Note: We don't cancel allocCtx here because we want it to live as long as the crawler.
	// In a real production app, we should manage this more carefully.
	c.ctx, c.cancel = chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))

	Logger.Info("browser will be navigated to codebeamer page, please login until 10 sec")
	err := chromedp.Run(c.ctx,
		chromedp.Navigate(c.config.CodebeamerHost),
		chromedp.Sleep(10*time.Second),
	)
	if err != nil {
		return err
	}

	if c.config.EnableCsrfToken {
		Logger.Info("fetch CSRF token for API compatibility")
		token, err := c.GetCsrfToken()
		if err != nil {
			return err
		}
		c.csrfToken = token
		Logger.WithField("value", token).Debug("csrf token updated")
	}

	return nil
}

func (c *ChromedpCrawler) GetCsrfToken() (string, error) {
	var token string
	err := chromedp.Run(c.ctx,
		chromedp.Evaluate(c.config.CsrfTokenExpression, &token),
	)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("cannot find token object")
	}
	return token, nil
}

func (c *ChromedpCrawler) FindRootTrackerByName(targetTrackerName string) (*RootTrackerNode, error) {
	Logger.WithFields(logrus.Fields{
		"targetName": targetTrackerName,
		"projectId":  c.config.FcuProjectId,
	}).Debug("FindRootTrackerByName")

	opt := createFetchOption("POST", false, nil, c.config.EnableCsrfToken, c.csrfToken)

	var result string
	err := chromedp.Run(c.ctx,
		chromedp.Navigate(c.config.CodebeamerHost),
		executeFetchInPage(
			fmt.Sprintf(c.config.GetTrackerHomePageTreeUrl, c.config.FcuProjectId),
			opt,
			&result,
		),
	)
	if err != nil {
		return nil, err
	}

	treeNodes := []RootTrackerNode{}
	if err := json.Unmarshal([]byte(result), &treeNodes); err != nil {
		return nil, err
	}

	tracker, found := lo.Find(treeNodes, func(item RootTrackerNode) bool {
		return item.Text == targetTrackerName
	})
	if !found {
		return nil, nil
	}

	return &tracker, nil
}

func (c *ChromedpCrawler) FillTrackerChild(targetTracker *TrackerNode) error {
	Logger.WithFields(logrus.Fields{
		"trackerId": targetTracker.Id,
	}).Debug("FillTrackerChild")

	err := chromedp.Run(c.ctx,
		chromedp.Navigate(fmt.Sprintf(c.config.CodebeamerHost+c.config.TrackerPageUrl, targetTracker.Id)),
		waitUntilJSVariableIsDefined(c.config.TreeConfigDataExpression, time.Duration(c.config.JsVariableWaitTimeout)*time.Second, 1*time.Second),
		chromedp.Evaluate(c.config.TreeConfigDataExpression, targetTracker),
	)
	if err != nil {
		return err
	}

	for _, issue := range targetTracker.Children {
		issue.AssertChild()
	}

	return nil
}

func (c *ChromedpCrawler) FillIssueChild(targetIssue *IssueNode, parentTrackerId string) error {
	Logger.WithFields(logrus.Fields{
		"issueId":   targetIssue.Id,
		"trackerId": parentTrackerId,
	}).Debug("FillIssueChild")

	if !targetIssue.HasChildren {
		return nil
	}

	opt := createFetchOption("POST", false, NewTrackerTreeRequest(parentTrackerId, c.config.FcuProjectId, targetIssue.Id, ""), c.config.EnableCsrfToken, c.csrfToken)

	var childString string
	err := chromedp.Run(c.ctx,
		executeFetchInPage(
			c.config.TreeAjaxUrl,
			opt,
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

func (c *ChromedpCrawler) FillIssueContent(issue *IssueNode) error {
	Logger.WithFields(logrus.Fields{
		"issueId": issue.Id,
	}).Debug("FillIssueContent")

	taskCtxTimeout, cancel := context.WithTimeout(c.ctx, time.Second*time.Duration(c.config.JsVariableWaitTimeout))
	defer cancel()

	var innerHTML []string
	err := chromedp.Run(taskCtxTimeout,
		chromedp.Navigate(fmt.Sprintf(c.config.CodebeamerHost+c.config.IssuePageUrl, issue.Id)),
		chromedp.WaitReady(c.config.IssueContentSelector, chromedp.ByQuery),
		getInnerHtmlBySelector(c.config.IssueContentSelector, &innerHTML),
	)

	if err != nil {
		return err
	}

	if len(innerHTML) < 1 {
		return fmt.Errorf("empty innerHTML")
	}

	issue.Content = innerHTML[0]
	return nil
}

func (c *ChromedpCrawler) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}
