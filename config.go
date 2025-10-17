package main

const (
	// 크롤링을 진행할 때 사용할 크롬 브라우저의 devtool 주소
	ChromeDevtoolsURL string = "ws://127.0.0.1:9222/devtools/browser"
	// 코드비머에서 트래커 정보를 받아올 API 주소
	GetTrackerHomePageTreeUrl string = "/cb/ajax/getTrackerHomePageTree.spr?proj_id=%s"
	// 코드비머에서 각 트래커 페이지의 주소
	TrackerPageUrl string = "/cb/tracker/%s"
	// 코드비머에서 트래커의 하위 아이템을 받아올 트리 APU 주소
	TreeAjaxUrl string = "/cb/trackers/ajax/tree.spr"

	// 사용자의 코드비머에 맞게 설정해야할 변수들
	// 이 설정은 HMC 코드비머에 맞춘 설정
	// 코드비머의 Host URL
	CodebeamerHost string = "https://ade-cb.hmckmc.co.kr"
	// FCU 대상 제어기, 대상 세대의 SW의 프로젝트 ID
	FcuProjectId string = "119"
	// SW 요구사항을 담은 최상위 노드의 이름
	FcuRequirementName string = "소프트웨어 요구사양 FCU"
	// 유효한 노드임을 나타내는지 판단하기 위한, 유효한 요구사항을 나타내는 아이콘 주소
	CodebeamerRqIconUrl string = "/cb/displayDocument?doc_id=30320010"
	// API의 반환값에서 트리 정보를 담는 변수 이름
	TreeConfigDataExpression string = "tree.config.data"
	// 실제 SW 사양을 포함하는 노드 이름
	RequirementNodeName string = "상세 사양"

	// 코드비머사에서 제공하는 Demo용 퍼블릭 코드비머에서 테스트를 위한 설정
	/*
		CodebeamerHost           string = "https://codebeamer.com"
		FcuProjectId             string = "1005"
		FcuRequirementName       string = "작업 항목"
		CodebeamerRqIconUrl      string = "/cb/displayDocument?doc_id=30320010"
		TreeConfigDataExpression string = "tree.config.data"
		RequirementNodeName      string = "downstream ex"
	*/
)
