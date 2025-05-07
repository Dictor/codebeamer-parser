package main

import "fmt"

type (
	TrackerTreeResponse struct {
		Id          string                `json:"id"`
		Title       string                `json:"title"`
		Text        string                `json:"text"`
		Level       int                   `json:"level"`
		HasChildren bool                  `json:"hasChildren"`
		Children    []TrackerTreeResponse `json:"children"`
		Icon        string                `json:"json"`
		TrackerId   int                   `json:"trakerId"`
		NodeId      int                   `json:"nodeId"`
		Url         string                `json:"url"`
	}
)

func NewTrackerTreeRequest(trackerId int, nodeId int, openNodes string) map[string]interface{} {
	return map[string]interface{}{
		"project_id":             FcuProjectId,
		"type":                   "",
		"tracker_id":             trackerId,
		"trackerId":              trackerId, // 실제 요청에서 이렇게 두개가 중복으로 있음
		"revision":               "",
		"view_id":                -11,
		"useOutlineCache":        true,
		"nodeId":                 nodeId,
		"ratingFilters":          []interface{}{},
		"dateFilters":            []interface{}{},
		"suspectedFilters":       []interface{}{},
		"statusFilters":          []interface{}{},
		"cbQL":                   fmt.Sprintf("project.id IN (%d) AND tracker.id IN (%d)", FcuProjectId, trackerId),
		"baselineModeBaselineId": "",
		"showAncestorItems":      true,
		"showDescendantItems":    false,
		"openNodes":              openNodes,
	}
}
