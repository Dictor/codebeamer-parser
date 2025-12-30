package main

type (
	ParsingConfig struct {
		// URL related options
		ChromeDevtoolsURL         string `mapstructure:"chrome_devtools_url" validate:"required,url"`
		CodebeamerHost            string `mapstructure:"codebeamer_host" validate:"required,url"`
		GetTrackerHomePageTreeUrl string `mapstructure:"get_tracker_home_page_tree_url" validate:"required"`
		TrackerPageUrl            string `mapstructure:"tracker_page_url" validate:"required"`
		IssuePageUrl              string `mapstructure:"issue_page_url" validate:"required"`
		TreeAjaxUrl               string `mapstructure:"tree_ajax_url" validate:"required,uri"`

		// detailed parsing options
		FcuProjectId                       string `mapstructure:"fcu_project_id" validate:"required"`
		FcuRequirementName                 string `mapstructure:"fcu_requirement_name" validate:"required"`
		CodebeamerRqIconUrl                string `mapstructure:"codebeamer_rq_icon_url" validate:"required,uri"`
		TreeConfigDataExpression           string `mapstructure:"tree_config_data_expression" validate:"required"`
		EnableRequirementNodeNameFiltering bool   `mapstructure:"enable_requirement_node_name_filtering"`
		RequirementNodeName                string `mapstructure:"requirement_node_name" validate:"required"`

		// API mechanism options
		IssueContentSelector  string `mapstructure:"issue_content_selector" validate:"required"`
		IntervalPerRequest    int    `mapstructure:"interval_per_request_ms" validate:"required"`
		JsVariableWaitTimeout int    `mapstructure:"js_variable_wait_timeout_s" validate:"required"`
		EnableCsrfToken       bool   `mapstructure:"enable_csrf_token"`
		CsrfTokenExpression   string `mapstructure:"csrf_token_expression" validate:"required"`
	}
)
