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
		"header":      map[string]string{},
		"body":        string(reqBody),
	}
	if isJson {
		req["header"].(map[string]string)["Content-Type"] = "application/json; charset=UTF-8"
	} else {
		req["header"].(map[string]string)["Content-Type"] = "application/x-www-form-urlencoded; charset=UTF-8"
	}
	return req
}

// getElementTextBySelector는 주어진 CSS 선택자에 해당하는 첫 번째 요소의 텍스트 내용을 가져옵니다.
// selector: 대상 요소를 찾는 CSS 선택자 문자열.
// textContent: 가져온 텍스트 내용을 저장할 문자열 포인터.
func getElementTextBySelector(selector string, textContent *string) chromedp.Action {
	log.Printf("CSS 선택자 '%s'로 요소 텍스트 가져오기 시도 중...", selector)
	// chromedp.Text는 지정된 선택자에 해당하는 첫 번째 요소의 텍스트를 가져옵니다.
	// chromedp.ByQuery는 CSS 선택자를 사용하여 요소를 찾도록 지정합니다.
	// chromedp.NodeVisible은 요소가 화면에 보일 때까지 기다립니다 (선택 사항이지만 유용함).
	return chromedp.Tasks{
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Text(selector, textContent, chromedp.ByQuery),
	}
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

	result := []TrackerTreeResponse{}
	if err := json.Unmarshal([]byte(trackerHomePageTreeResult), &result); err != nil {
		log.Panicf("[Step 1] 결과 파싱 오류: %v", err)
	}
	log.Printf("[Step 1] 총 %d개의 노드를 프로젝트에서 가져옴: %v", len(result), lo.Map(result, func(item TrackerTreeResponse, index int) string {
		return item.Text
	}))

	requirementNode, requirementNodeFound := lo.Find(result, func(item TrackerTreeResponse) bool {
		return item.Text == FcuRequirementName
	})
	if !requirementNodeFound {
		log.Panicf("[Step 1] 요구사항 노드 '%s'를 찾지 못함", FcuRequirementName)
	}

	requirementChildNodes := lo.Filter(requirementNode.Children, func(item TrackerTreeResponse, index int) bool {
		return true
		//return item.Icon == CodebeamerRqIconUrl
	})
	log.Printf("[Step 1] 요구사항 노드에서 총 %d의 하위 사양 노드 발견", len(requirementChildNodes))

	// < Step 2. 각 사양문서 파싱 >
	// 5. 각 사양 트래커 페이지로 크롬 탐색
	// 6. 페이지에서 트리 설명 js 객체 조회
	for _, node := range requirementChildNodes {
		log.Printf("[Step 2] 하위 사양 노드 ID '%s' 조회 시작", node.Id)
		trackerId, err := strconv.Atoi(node.Id)
		if err != nil {
			log.Printf("[Step 2] 하위 사양 노드 ID '%s'가 올바르지 않음", node.Id)
			continue
		}

		treeConfigData := TrackerTreeResponse{}
		err = chromedp.Run(taskCtx,
			chromedp.Navigate(fmt.Sprintf(CodebeamerHost+TrackerPageUrl, trackerId)),
			waitUntilJSVariableIsDefined(TreeConfigDataExpression, 10*time.Second, 1*time.Second),
			chromedp.Evaluate(TreeConfigDataExpression, &treeConfigData),
		)
		if err != nil {
			log.Printf("[Step 2] 크롬 오류 발생: %v", err)
			continue
		}
		log.Printf("[Step 2] 하위 사양 노드 ID '%s' 조회 완료, 하위 노드 %d개", node.Id, len(treeConfigData.Children))

		// < Step 3. 각 사양문서의 자식 파싱 >
		var searchFunc func(*TrackerTreeResponse, *TrackerTreeResponse) (bool, error)
		searchFunc = func(node *TrackerTreeResponse, searchResult *TrackerTreeResponse) (bool, error) {
			lvNodeData := ""
			err := chromedp.Run(taskCtx,
				chromedp.Sleep(1*time.Second),
				executeFetchInPage(
					TreeAjaxUrl,
					createFetchOption("POST", false, NewTrackerTreeRequest(trackerId, node.NodeId, "")),
					&lvNodeData,
				),
			)
			if err != nil {
				log.Printf("[Step 3] 크롬 오류 발생: %v", err)
				return false, err
			}
			result := []TrackerTreeResponse{}
			if err := json.Unmarshal([]byte(lvNodeData), &result); err != nil {
				log.Printf("[Step 3] 결과 파싱 오류: %v", err)
				return false, err
			}

			// 일단 서버 부하가 적은 Text 비교부터 수행
			for _, lv3Node := range result {
				if lv3Node.Text == RequirementNodeName {
					*searchResult = lv3Node
					return true, nil
				}
			}

			for _, lv3Node := range result {
				found, _ := searchFunc(&lv3Node, searchResult)
				if found {
					return true, nil
				}
			}

			return false, nil
		}

		treeConfigDataSearchResult := TrackerTreeResponse{}
		for _, lv2Node := range treeConfigData.Children {
			found, _ := searchFunc(&lv2Node, &treeConfigDataSearchResult)
			if found {
				log.Printf("[Step 2] 하위 사양 노드 ID '%s'의 RQ 검색 성공", treeConfigData.Id)
				break
			}
		}
	}

	log.Println("작업 완료, Enter키를 누르면 프로그램이 종료됩니다.")
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}
