package main

import (
	"bufio"
	"context"
	"encoding/json" // JSON 처리를 위해 추가
	"fmt"
	"log" // os.Executable 사용 위해 추가
	"net/url"
	"os"
	"strconv"
	"strings"

	// 경로 조작 위해 추가
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/samber/lo"
)

// --- Functions ---
func createFetchOption(httpMethod string, isJson bool, body map[string]interface{}) map[string]interface{} {
	var (
		reqBody []byte = []byte{}
		err     error
	)

	if body != nil {
		if isJson { // json
			reqBody, err = json.Marshal(body)
			if err != nil {
				panic(err)
			}
		} else { // form
			values := url.Values{}
			for key, value := range body {
				var valueStr string
				if value == nil {
					valueStr = ""
				} else {
					valueStr = fmt.Sprintf("%v", value)
				}
				values.Add(key, valueStr)
			}
			reqBody = []byte(values.Encode())
		}
	}

	req := map[string]interface{}{
		"method":      httpMethod,
		"mode":        "cors",
		"cache":       "default",
		"credentials": "include",
		"headers":     map[string]string{},
		"body":        string(reqBody),
	}
	if isJson {
		req["headers"].(map[string]string)["Content-Type"] = "application/json; charset=UTF-8"
	} else {
		req["headers"].(map[string]string)["Content-Type"] = "application/x-www-form-urlencoded; charset=UTF-8"
	}
	return req
}

// executeFetchInPage는 페이지 컨텍스트 내에서 JavaScript fetch API를 실행하고 결과를 가져옵니다.
// fetchURL: fetch 요청을 보낼 URL.
// options: fetch 요청에 사용할 옵션 (method, headers, body 등). nil이면 기본 GET 요청.
// fetchResult: fetch 요청의 응답 텍스트를 저장할 문자열 포인터.
func executeFetchInPage(fetchURL string, options map[string]interface{}, fetchResult *string) chromedp.Action {
	log.Printf("페이지 내에서 '%s'로 fetch 요청 실행 중 (옵션 포함)...", fetchURL)

	// 옵션 map을 JSON 문자열로 변환
	var optionsJSON string
	if options != nil {
		jsonData, err := json.Marshal(options)
		if err != nil {
			// 실제 프로덕션 코드에서는 에러 처리를 더 견고하게 해야 합니다.
			log.Printf("경고: fetch 옵션을 JSON으로 변환하는 데 실패했습니다: %v. 기본 GET 요청을 사용합니다.", err)
			optionsJSON = "{}" // 빈 객체로 설정하여 기본 GET 요청처럼 동작
		} else {
			optionsJSON = string(jsonData)
		}
	} else {
		optionsJSON = "{}" // 옵션이 nil이면 빈 객체 사용
	}

	// JavaScript 코드를 생성합니다. fetch 후 응답을 텍스트로 변환하여 반환합니다.
	// 두 번째 인수로 optionsJSON을 전달합니다.
	script := fmt.Sprintf(`
		fetch('%s', %s) // 옵션 객체를 두 번째 인수로 전달
			.then(response => {
				if (!response.ok) {
					// 응답 상태 코드와 텍스트를 포함하여 더 자세한 오류 메시지 제공
					return response.text().then(text => {
						throw new Error("Network response was not ok.");
					});
				}
				return response.text(); // 또는 response.json() 등 필요에 따라 변경
			})
			.then(data => {
				return data; // 최종 결과를 반환
			})
			.catch(error => {
				console.error('Fetch error:', error);
				// 오류 객체에서 message 속성을 가져오도록 수정
				return 'Fetch Error: ' + (error.message || String(error)); // 오류 메시지 반환
			});
	`, fetchURL, optionsJSON) // optionsJSON을 포맷 문자열에 추가

	// chromedp.Evaluate는 페이지에서 JavaScript 코드를 실행하고 결과를 반환받습니다.
	// awaitPromise 플래그는 JavaScript Promise가 완료될 때까지 기다립니다.
	return chromedp.Evaluate(script, fetchResult, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	})
}

// waitUntilJSVariableIsDefined는 특정 JS 변수가 정의될 때까지 기다리는 Action을 반환합니다.
// 이 Action은 chromedp.Run 시퀀스 내에서 사용될 수 있습니다.
// variableName: 기다릴 전역 변수의 이름 (예: "myGlobalVar")
// timeout: 최대 대기 시간
// interval: 존재 여부를 확인할 간격
func waitUntilJSVariableIsDefined(variableName string, timeout, interval time.Duration) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// 변수 존재 여부를 확인하는 JavaScript 스크립트
		checkScript := fmt.Sprintf("typeof window.%s !== 'undefined'", variableName)
		// Optional: Wait for the variable to be truthy
		// checkScript := fmt.Sprintf("!!window.%s", variableName)
		// Or a more complex condition:
		// checkScript := "window.someObject && window.someObject.someProperty === 'ready'" // Ensure this returns a boolean

		var exists bool // JavaScript 평가 결과를 저장할 변수 (boolean)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-timeoutCtx.Done():
				return fmt.Errorf("ActionFunc: timed out waiting for JavaScript condition based on variable %q: %w", variableName, timeoutCtx.Err())
			case <-ticker.C:
				err := chromedp.Run(timeoutCtx,
					chromedp.Evaluate(checkScript, &exists),
				)

				if err != nil {
					continue
				}

				if exists {
					return nil
				}
			}
		}
	})
}

// --- Main Execution ---

func main() {
	log.Printf("실행 중인 Chrome 브라우저(%s)에 연결", ChromeDevtoolsURL)

	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), ChromeDevtoolsURL)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancelTask()

	log.Println("자동으로 코드비머 페이지로 이동합니다, 30초안에 로그인을 진행해주세요.")

	// < Step 1. 전체 구조 파싱 >
	// 1. 코드비머 페이지로 이동
	// 2. 30초 대기
	// 3. /cb/ajax/getTrackerHomePageTree.spr 요청 진행하여 프로젝트 정보 받아오기
	log.Print("[Step 1] 프로젝트 정보 파싱")
	trackerHomePageTreeResult := ""

	err := chromedp.Run(taskCtx,
		chromedp.Navigate(CodebeamerHost),
		chromedp.Sleep(5*time.Second),
		executeFetchInPage(
			fmt.Sprintf(GetTrackerHomePageTreeUrl, FcuProjectId),
			createFetchOption("POST", false, nil),
			&trackerHomePageTreeResult,
		),
	)
	if err != nil {
		log.Panicf("[Step 1] 크롬 오류 발생: %v", err)
	}

	result := []TreeType1Response{}
	if err := json.Unmarshal([]byte(trackerHomePageTreeResult), &result); err != nil {
		log.Panicf("[Step 1] 결과 파싱 오류: %v", err)
	}
	log.Printf("[Step 1] 총 %d개의 노드를 프로젝트에서 가져옴: %v", len(result), lo.Map(result, func(item TreeType1Response, index int) string {
		return item.Text
	}))

	requirementRoot, requirementRootFound := lo.Find(result, func(item TreeType1Response) bool {
		return item.Text == FcuRequirementName
	})
	if !requirementRootFound {
		log.Panicf("[Step 1] 요구사항 루트 노드 '%s'를 찾지 못함", FcuRequirementName)
	}

	requirementNodes := lo.Filter(requirementRoot.Children, func(item TreeType1Response, index int) bool {
		return item.Icon == CodebeamerRqIconUrl
	})
	log.Printf("[Step 1] 요구사항 루트 노드에서 총 %d의 사양 노드 발견", len(requirementNodes))

	// < Step 2. 각 사양 노드 파싱 >
	// 5. 각 사양 트래커 페이지로 크롬 탐색
	// 6. 페이지에서 트리 설명 js 객체 조회
	type logResult struct {
		DetailRqId string
		DetailRq   *TreeType2Child
		Complexity int
	}
	totalLogResult := []logResult{}

	for _, node := range requirementNodes {
		log.Printf("[Step 2] 사양 노드 ID '%s' 조회 시작", node.Id)
		trackerId, err := strconv.Atoi(node.Id)
		if err != nil {
			log.Printf("[Step 2] 사양 노드 ID '%s'가 올바르지 않음", node.Id)
			continue
		}

		treeConfigData := TreeType2Response{}
		err = chromedp.Run(taskCtx,
			chromedp.Navigate(fmt.Sprintf(CodebeamerHost+TrackerPageUrl, trackerId)),
			waitUntilJSVariableIsDefined(TreeConfigDataExpression, 10*time.Second, 1*time.Second),
			chromedp.Evaluate(TreeConfigDataExpression, &treeConfigData),
		)
		if err != nil {
			log.Printf("[Step 2] 크롬 오류 발생: %v", err)
			continue
		}

		log.Printf("[Step 2] 사양 노드 ID '%s' 조회 완료, 하위 문서 %d개", node.Id, len(treeConfigData.Children))

		// < Step 3. 각 사양 노드의 하위 문서 파싱 >
		findResult := []*TreeType2Child{}
		for _, doc := range treeConfigData.Children {
			time.Sleep(300 * time.Millisecond)
			recursiveFindDoc(taskCtx, trackerId, &doc, &findResult)
		}
		log.Printf("[Step 3] 총 %d개의 상세 사양 문서 찾음", len(findResult))

		// < Step 4. 각 상세 사양 문서의 복잡도 계산 >
		for _, detailRq := range findResult {
			complexity := getComplexityOfDetailRQ(taskCtx, trackerId, detailRq)
			log.Printf("[Step 4] 상세 사양 문서 ID=%d의 복잡도=%d", detailRq.NodeId, complexity)
			totalLogResult = append(totalLogResult, logResult{
				DetailRqId: detailRq.Id,
				DetailRq:   detailRq,
				Complexity: complexity,
			})
		}
	}

	tlog, err := json.Marshal(totalLogResult)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("result.json", tlog, 0777); err != nil {
		panic(err)
	}

	log.Println("작업 완료, Enter키를 누르면 프로그램이 종료됩니다.")
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}

func getComplexityOfDetailRQ(chromeCtx context.Context, trackerId int, detailRQ *TreeType2Child) int {
	log.Printf("[getComplexityOfDetailRQ] 상세 사양 탐색 시작작, 문서 ID=%s", detailRQ.Id)

	drqContent := ""
	err := chromedp.Run(chromeCtx,
		chromedp.Sleep(300*time.Millisecond),
		executeFetchInPage(
			TreeAjaxUrl,
			createFetchOption("POST", false, NewTrackerTreeRequest(trackerId, detailRQ.NodeId, "")),
			&drqContent,
		),
	)
	if err != nil {
		log.Printf("[recursiveFindDoc] 크롬 오류: %v", err)
		return 0
	}
	drqElements := []TreeType3Child{}
	if err := json.Unmarshal([]byte(drqContent), &drqElements); err != nil {
		log.Printf("[recursiveFindDoc] 파싱 오류: %v", err)
		return 0
	}

	ret := 0
	for _, element := range drqElements {
		if element.ListAttr.IconBgColor == "#ababab" {
			continue
		}
		ret += strings.Count(element.Text, "[ISSUE:")
	}

	return ret
}

func recursiveFindDoc(chromeCtx context.Context, trackerId int, doc *TreeType2Child, result *[]*TreeType2Child) bool {
	log.Printf("[recursiveFindDoc] 문서 탐색 시작, 문서 ID=%s", doc.Id)

	docContent := ""
	err := chromedp.Run(chromeCtx,
		chromedp.Sleep(300*time.Millisecond),
		executeFetchInPage(
			TreeAjaxUrl,
			createFetchOption("POST", false, NewTrackerTreeRequest(trackerId, doc.NodeId, "")),
			&docContent,
		),
	)
	if err != nil {
		log.Printf("[recursiveFindDoc] 크롬 오류: %v", err)
		return false
	}
	docElements := []TreeType2Child{}
	if err := json.Unmarshal([]byte(docContent), &docElements); err != nil {
		log.Printf("[recursiveFindDoc] 파싱 오류: %v", err)
		return false
	}

	// 일단 서버 부하가 적은 Text 비교부터 수행
	for _, element := range docElements {
		if element.Text == RequirementNodeName {
			log.Printf("[recursiveFindDoc] RQ 요소 탐색 성공, 문서 ID=%s의 하위 요소 ID=%s", doc.Id, element.Id)
			*result = append(*result, &element)
			return true // 상세 사양을 찾았으면 해당 doc에 대해서 더 이상 찾지 않아도 됨
		}
	}

	for _, element := range docElements {
		time.Sleep(time.Second)
		if recursiveFindDoc(chromeCtx, trackerId, &element, result) {
			return true
		}
	}

	log.Printf("[recursiveFindDoc] 문서 탐색 종료, 문서 ID=%s", doc.Id)
	return false
}
