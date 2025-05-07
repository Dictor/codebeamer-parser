package main

import "fmt"

type (
	TreeType1Response struct {
		Id          string              `json:"id"`
		Title       string              `json:"title"`
		Text        string              `json:"text"`
		Level       int                 `json:"level"`
		HasChildren bool                `json:"hasChildren"`
		Children    []TreeType1Response `json:"children"`
		Icon        string              `json:"icon"`
		TrackerId   int                 `json:"trakerId"`
		NodeId      int                 `json:"nodeId"`
		Url         string              `json:"url"`
	}

	TreeType2Response struct {
		Id          string           `json:"id"`
		Title       string           `json:"title"`
		Text        string           `json:"text"`
		Level       int              `json:"level"`
		HasChildren bool             `json:"hasChildren"`
		Children    []TreeType2Child `json:"children"`
		Icon        string           `json:"icon"`
		TrackerId   int              `json:"trakerId"`
		NodeId      int              `json:"nodeId"`
		Url         string           `json:"url"`
	}

	TreeType2Child struct {
		Id          string `json:"id"`
		Title       string `json:"title"`
		Text        string `json:"text"`
		Level       int    `json:"level"`
		HasChildren bool   `json:"hasChildren"`
		Children    bool   `json:"children"`
		Icon        string `json:"icon"`
		TrackerId   int    `json:"trakerId"`
		NodeId      int    `json:"nodeId"`
		Url         string `json:"url"`
	}

	TreeType3Child struct {
		Id          string        `json:"id"`
		Title       string        `json:"title"`
		Text        string        `json:"text"`
		Level       int           `json:"level"`
		HasChildren bool          `json:"hasChildren"`
		Children    []interface{} `json:"children"`
		Icon        string        `json:"icon"`
		TrackerId   int           `json:"trakerId"`
		NodeId      int           `json:"nodeId"`
		Url         string        `json:"url"`
		ListAttr    struct {
			IconBgColor string `json:"iconBgColor"`
		} `json:"li_attr"`
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
