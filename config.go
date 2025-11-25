package main

type (
	ParsingConfig struct {
		ChromeDevtoolsURL         string `mapstructure:"chrome_devtools_url" validate:"required,url"`
		GetTrackerHomePageTreeUrl string `mapstructure:"get_tracker_home_page_tree_url" validate:"required"`
		TrackerPageUrl            string `mapstructure:"tracker_page_url" validate:"required"`
		TreeAjaxUrl               string `mapstructure:"tree_ajax_url" validate:"required,uri"`
		CodebeamerHost            string `mapstructure:"codebeamer_host" validate:"required,url"`
		FcuProjectId              string `mapstructure:"fcu_project_id" validate:"required"`
		FcuRequirementName        string `mapstructure:"fcu_requirement_name" validate:"required"`
		CodebeamerRqIconUrl       string `mapstructure:"codebeamer_rq_icon_url" validate:"required,uri"`
		TreeConfigDataExpression  string `mapstructure:"tree_config_data_expression" validate:"required"`
		RequirementNodeName       string `mapstructure:"requirement_node_name" validate:"required"`
		IntervalPerRequest        int    `mapstructure:"interval_per_request_ms" validate:"required"`
		JsVariableWaitTimeout     int    `mapstructure:"js_variable_wait_timeout_s" validate:"required"`
	}
)

const (
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
