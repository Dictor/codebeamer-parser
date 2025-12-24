package main

import (
	"fmt"
)

type (
	// 트래커의 형식입니다. 각각은 코드비머에서 하나의 대주제를 나타냅니다.
	Tracker struct {
		Id        string `json:"id"`
		TrackerId int    `json:"trackerId"`
		Title     string `json:"title"`
		Text      string `json:"text"`
		Icon      string `json:"icon"`
		Url       string `json:"url"`
	}

	// 최상위 트래커의 인스턴스 형식입니다.
	// 전체 프로젝트에서 하나만 존재할 것으로 예상되며 자식 트래커를 가집니다.
	RootTrackerNode struct {
		Tracker
		Children []*TrackerNode `json:"children"`
	}

	// 트래커의 인스턴스 형식입니다.
	// 최상위 트래커의 자식일 것으로 예상되며 자식 이슈를 가집니다.
	TrackerNode struct {
		Tracker
		Children  []*IssueNode `json:"children"`
		GraphNode interface{}  `json:"-"`
	}

	// 이슈의 인스턴스 형식입니다.
	// 최하위 요소로, 실제 사양 정의를 담고 있습니다.
	// 이슈는 자식 이슈를 가질 수 있습니다.
	IssueNode struct {
		Id       string      `json:"id"`
		Title    string      `json:"title"`
		Content  string      `json:"content"`
		Text     string      `json:"text"`
		Icon     string      `json:"icon"`
		Url      string      `json:"url"`
		Children interface{} `json:"children"`
		ListAttr struct {
			IconBgColor string `json:"iconBgColor"`
		} `json:"li_attr"`
		HasChildren  bool
		RealChildren []*IssueNode
	}
)

// 트래커 트리를 얻기 위한 API 요청 객체를 생성
func NewTrackerTreeRequest(trackerId string, FcuProjectId string, nodeId string, openNodes string) map[string]interface{} {
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
		"cbQL":                   fmt.Sprintf("project.id IN (%s) AND tracker.id IN (%s)", FcuProjectId, trackerId),
		"baselineModeBaselineId": "",
		"showAncestorItems":      true,
		"showDescendantItems":    false,
		"openNodes":              openNodes,
	}
}

// 이슈가 실제 자식 이슈를 가지는지 단언
func (i *IssueNode) AssertChild() {
	i.HasChildren = false
	if i.Children == nil {
		return
	}

	switch v := i.Children.(type) {
	case bool:
		if v {
			i.HasChildren = true
		}
	case []interface{}:
		return
	default:
		return
	}
}
