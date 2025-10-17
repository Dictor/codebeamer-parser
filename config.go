package main

const (
	ChromeDevtoolsURL         string = "ws://127.0.0.1:9222/devtools/browser"
	GetTrackerHomePageTreeUrl string = "/cb/ajax/getTrackerHomePageTree.spr?proj_id=%s"
	TrackerPageUrl            string = "/cb/tracker/%s"
	TreeAjaxUrl               string = "/cb/trackers/ajax/tree.spr"

	// user variant setting
	// settings for HMC
	CodebeamerHost           string = "https://ade-cb.hmckmc.co.kr"
	FcuProjectId             string = "119"
	FcuRequirementName       string = "소프트웨어 요구사양 FCU"
	CodebeamerRqIconUrl      string = "/cb/displayDocument?doc_id=30320010"
	TreeConfigDataExpression string = "tree.config.data"
	RequirementNodeName      string = "상세 사양"

	// settings for PTC
	/*
		CodebeamerHost           string = "https://codebeamer.com"
		FcuProjectId             string = "1005"
		FcuRequirementName       string = "작업 항목"
		CodebeamerRqIconUrl      string = "/cb/displayDocument?doc_id=30320010"
		TreeConfigDataExpression string = "tree.config.data"
		RequirementNodeName      string = "downstream ex"
	*/
)
