package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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
			Timeout: 60 * time.Second,
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
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

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
		Logger.WithFields(logrus.Fields{
			"status": resp.Status,
			"url":    url,
		}).Debug("REST API response received")
	}

	return resp, nil
}

func (c *RestCrawler) Login() error {
	Logger.Info("verifying REST API credentials and project access")
	url := fmt.Sprintf("%s/cb/api/v3/projects/%s", c.config.CodebeamerHost, c.config.FcuProjectId)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		Logger.Info("REST API credentials and project access verified")
		return nil
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("REST API login failed: Invalid username or password (401)")
	case http.StatusForbidden:
		return fmt.Errorf("REST API login failed: Insufficient permissions for project %s (403)", c.config.FcuProjectId)
	case http.StatusNotFound:
		return fmt.Errorf("REST API login failed: Project ID %s not found (404)", c.config.FcuProjectId)
	default:
		return fmt.Errorf("REST API login failed: unexpected status code %d", resp.StatusCode)
	}
}

type trackerResponse struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

func (c *RestCrawler) FindRootTrackerByName(name string) (*RootTrackerNode, error) {
	url := fmt.Sprintf("%s/cb/api/v3/projects/%s/trackers", c.config.CodebeamerHost, c.config.FcuProjectId)
	
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result []trackerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var target *trackerResponse
	for i := range result {
		if strings.TrimSpace(result[i].Name) == strings.TrimSpace(name) {
			target = &result[i]
			break
		}
	}

	if target == nil {
		return nil, fmt.Errorf("root tracker not found: %s", name)
	}

	root := &RootTrackerNode{
		Tracker: Tracker{
			Id:        "work",
			TrackerId: 0,
			Text:      target.Name,
		},
		Children: []*TrackerNode{
			{
				Tracker: Tracker{
					Id:        fmt.Sprintf("%d-tracker", target.Id),
					TrackerId: target.Id,
					Text:      target.Name,
				},
			},
		},
	}

	return root, nil
}

type paginationResponse struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
	ItemRefs []struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	} `json:"itemRefs"`
}

func (c *RestCrawler) FillTrackerChild(tracker *TrackerNode) error {
	Logger.WithField("trackerId", tracker.TrackerId).Info("fetching tracker children")

	pageSize := 100
	page := 1
	var allItems []struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	}

	for {
		url := fmt.Sprintf("%s/cb/api/v3/trackers/%d/children?page=%d&pageSize=%d", c.config.CodebeamerHost, tracker.TrackerId, page, pageSize)
		resp, err := c.doRequest("GET", url, nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to fetch tracker children: %d", resp.StatusCode)
		}

		var paginated paginationResponse
		if err := json.NewDecoder(resp.Body).Decode(&paginated); err != nil {
			return err
		}

		allItems = append(allItems, paginated.ItemRefs...)

		if len(allItems) >= paginated.Total || len(paginated.ItemRefs) == 0 {
			break
		}
		page++
	}

	tracker.Children = make([]*IssueNode, 0, len(allItems))
	for _, item := range allItems {
		node := &IssueNode{
			Id:    strconv.Itoa(item.Id),
			Title: item.Name,
			Text:  item.Name,
		}
		node.AssertChild()
		tracker.Children = append(tracker.Children, node)
	}

	tracker.Url = fmt.Sprintf("/tracker/%d", tracker.TrackerId)
	Logger.WithFields(logrus.Fields{
		"trackerId": tracker.TrackerId,
		"total":     len(allItems),
	}).Info("tracker children fetched")

	return nil
}




type itemField struct {
	Name   string      `json:"name"`
	Value  interface{} `json:"value"`
	Values []struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	} `json:"values"`
}

type itemFieldsResponse struct {
	EditableFields []itemField `json:"editableFields"`
	ReadOnlyFields []itemField `json:"readOnlyFields"`
}

type itemResponse struct {
	IconUrl     string `json:"iconUrl"`
	IconColor   string `json:"iconColor"`
	Description string `json:"description"`
}

func (c *RestCrawler) formatIconUrl(url string) string {
	if url == "" {
		return ""
	}
	if strings.HasPrefix(url, "/cb/") {
		return url
	}
	// REST API는 /images/... 형태이므로 /cb를 붙여줌
	if strings.HasPrefix(url, "/") {
		return "/cb" + url
	}
	return "/cb/" + url
}

func (c *RestCrawler) FillIssueChild(issue *IssueNode, parentTrackerId string) error {
	Logger.WithField("issueId", issue.Id).Info("fetching issue children")
	url := fmt.Sprintf("%s/cb/api/v3/items/%s/fields", c.config.CodebeamerHost, issue.Id)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch issue fields: %d", resp.StatusCode)
	}

	var fields itemFieldsResponse
	if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
		return err
	}

	issue.RealChildren = []*IssueNode{}
	// Combine both editable and read-only fields to search for "Children"
	allFields := append(fields.EditableFields, fields.ReadOnlyFields...)
	for _, f := range allFields {
		if f.Name == "Children" {
			for _, childRef := range f.Values {
				childNode := &IssueNode{
					Id:    strconv.Itoa(childRef.Id),
					Title: childRef.Name,
					Text:  childRef.Name,
				}
				childNode.AssertChild()
				issue.RealChildren = append(issue.RealChildren, childNode)
			}
			break
		}
	}

	return nil
}

func (c *RestCrawler) FillIssueContent(issue *IssueNode) error {
	Logger.WithField("issueId", issue.Id).Info("fetching issue content")
	
	// Step 4 mentions /items/{itemId}/field for icon and /items/{itemId}/fields for Description.
	// However, GET /items/{itemId} provides both iconUrl and description directly.
	url := fmt.Sprintf("%s/cb/api/v3/items/%s", c.config.CodebeamerHost, issue.Id)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch item details: %d", resp.StatusCode)
	}

	var item itemResponse
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return err
	}

	issue.Content = item.Description
	issue.Icon = c.formatIconUrl(item.IconUrl)
	issue.ListAttr.IconBgColor = item.IconColor
	issue.Url = fmt.Sprintf("/item/%s", issue.Id)

	return nil
}


func (c *RestCrawler) Close() error {
	return nil
}
