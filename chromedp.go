package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// createFetchOption은 fetch 요청에 필요한 옵션 객체를 생성합니다.
// httpMethod: HTTP 메서드 (GET, POST 등)
// isJson: 요청 본문이 JSON 형식인지 여부
// body: 요청 본문에 포함될 데이터 (map[string]interface{} 형식)
func createFetchOption(httpMethod string, isJson bool, body map[string]interface{}, enableCsrf bool, csrfToken string) map[string]interface{} {
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

	headers := req["headers"].(map[string]string)
	if enableCsrf {
		headers["x-csrf-token"] = csrfToken
		headers["X-Requested-With"] = "XMLHttpRequest" // 서버가 AJAX 요청으로 식별하기 위해 보통 필수
	}

	return req
}

// executeFetchInPage는 페이지 컨텍스트 내에서 JavaScript fetch API를 실행하고 결과를 가져옵니다.
// fetchURL: fetch 요청을 보낼 URL.
// options: fetch 요청에 사용할 옵션 (method, headers, body 등). nil이면 기본 GET 요청.
// fetchResult: fetch 요청의 응답 텍스트를 저장할 문자열 포인터.
func executeFetchInPage(fetchURL string, options map[string]interface{}, fetchResult *string) chromedp.Action {
	var optionsJSON string
	if options != nil {
		jsonData, err := json.Marshal(options)
		if err != nil {
			// 실제 프로덕션 코드에서는 에러 처리를 더 견고하게 해야 합니다.
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

// getInnerHtmlBySelector는 주어진 CSS 선택자에 해당하는 모든 요소의 innerHTML을 가져오는 Action을 반환합니다.
// selector: innerHTML을 가져올 요소를 찾기 위한 CSS 선택자.
// results: innerHTML 문자열 슬라이스를 저장할 포인터.
func getInnerHtmlBySelector(selector string, results *[]string) chromedp.Action {
	script := fmt.Sprintf(`
		Array.from(document.querySelectorAll('%s')).map(element => element.innerHTML);
	`, selector)

	return chromedp.Evaluate(script, results)
}
