package main

import (
	"fmt"
)

// Crawler defines the interface for interacting with Codebeamer to fetch data.
type Crawler interface {
	// Login handles the initial authentication or connection setup.
	Login() error
	// FindRootTrackerByName searches for a root-level tracker by its display name.
	FindRootTrackerByName(name string) (*RootTrackerNode, error)
	// FillTrackerChild populates the children of a given tracker.
	FillTrackerChild(tracker *TrackerNode) error
	// FillIssueChild populates the direct children of a given issue.
	FillIssueChild(issue *IssueNode, parentTrackerId string) error
	// FillIssueContent fetches the detailed content (e.g., wiki description) of an issue.
	FillIssueContent(issue *IssueNode) error
	// Close cleans up any resources used by the crawler.
	Close() error
}

// NewCrawler is a factory function that returns the appropriate Crawler implementation.
func NewCrawler(crawlerType string, config ParsingConfig) (Crawler, error) {
	switch crawlerType {
	case "chromedp":
		return NewChromedpCrawler(config), nil
	case "rest":
		// TODO: Implement REST API crawler
		return nil, fmt.Errorf("REST API crawler not implemented yet")
	default:
		return nil, fmt.Errorf("unknown crawler type: %s", crawlerType)
	}
}
