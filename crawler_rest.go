package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

type RestCrawler struct {
	config     ParsingConfig
	httpClient *http.Client
	authHeader string
}

func NewRestCrawler(config ParsingConfig) *RestCrawler {
	auth := config.Username + ":" + config.Password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	return &RestCrawler{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		authHeader: "Basic " + encodedAuth,
	}
}

func (c *RestCrawler) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader)

	if Logger.GetLevel() >= logrus.DebugLevel {
		Logger.WithFields(logrus.Fields{
			"method": method,
			"url":    url,
		}).Debug("REST API request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if Logger.GetLevel() >= logrus.DebugLevel {
		// Log response status
		Logger.WithFields(logrus.Fields{
			"status": resp.Status,
			"url":    url,
		}).Debug("REST API response received")

		// Optionally log body if it's not too large
		if resp.Body != nil {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore body for later use
			
			bodyStr := string(bodyBytes)
			if len(bodyStr) > 1000 {
				bodyStr = bodyStr[:1000] + "..."
			}
			Logger.WithField("body", bodyStr).Debug("REST API response body (truncated)")
		}
	}

	return resp, nil
}

func (c *RestCrawler) Login() error {
	Logger.Info("verifying REST API credentials")
	resp, err := c.doRequest("GET", c.config.CodebeamerHost+"/cb/api/v3/users/self", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("REST API login failed: status %d", resp.StatusCode)
	}

	Logger.Info("REST API credentials verified")
	return nil
}

func (c *RestCrawler) FindRootTrackerByName(name string) (*RootTrackerNode, error) {
	url := fmt.Sprintf("%s/cb/api/v3/projects/%s/trackers", c.config.CodebeamerHost, c.config.FcuProjectId)
	
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Trackers []struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		} `json:"trackers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	for _, t := range result.Trackers {
		if t.Name == name {
			return &RootTrackerNode{
				Tracker: Tracker{
					Id:        strconv.Itoa(t.Id),
					TrackerId: t.Id,
					Text:      t.Name,
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("root tracker not found: %s", name)
}

func (c *RestCrawler) FillTrackerChild(tracker *TrackerNode) error {
	url := fmt.Sprintf("%s/cb/api/v3/trackers/%d/items", c.config.CodebeamerHost, tracker.TrackerId)
	
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Items []struct {
			Id     int    `json:"id"`
			Name   string `json:"name"`
			Parent struct {
				Id int `json:"id"`
			} `json:"parent"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	tracker.Children = []*IssueNode{}
	for _, item := range result.Items {
		if item.Parent.Id == 0 {
			issue := &IssueNode{
				Id:    strconv.Itoa(item.Id),
				Title: item.Name,
				Text:  item.Name,
			}
			tracker.Children = append(tracker.Children, issue)
		}
	}

	return nil
}

func (c *RestCrawler) FillIssueChild(issue *IssueNode, parentTrackerId string) error {
	url := fmt.Sprintf("%s/cb/api/v3/items/%s/children", c.config.CodebeamerHost, issue.Id)
	
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		issue.RealChildren = []*IssueNode{}
		issue.HasChildren = false
		return nil
	}

	var result struct {
		ItemRefs []struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		} `json:"itemRefs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	issue.RealChildren = []*IssueNode{}
	for _, ref := range result.ItemRefs {
		child := &IssueNode{
			Id:    strconv.Itoa(ref.Id),
			Title: ref.Name,
			Text:  ref.Name,
		}
		issue.RealChildren = append(issue.RealChildren, child)
	}
	issue.HasChildren = len(issue.RealChildren) > 0

	return nil
}

func (c *RestCrawler) FillIssueContent(issue *IssueNode) error {
	url := fmt.Sprintf("%s/cb/api/v3/items/%s", c.config.CodebeamerHost, issue.Id)
	
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	issue.Content = result.Description
	return nil
}

func (c *RestCrawler) Close() error {
	return nil
}
