package main

import (
	"bufio"
	"context"
	"encoding/json" // JSON 처리를 위해 추가
	"fmt"
	"log" // os.Executable 사용 위해 추가
	"os"

	// 경로 조작 위해 추가
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// --- Functions ---
func createFetchOption(httpMethod string, jsonBody map[string]interface{}) map[string]interface{} {
	jsonBodyString := ""
	if jsonBody != nil {
		jsonBodyString, err := json.Marshal(jsonBody)
		if err != nil {
			panic(err)
		}
	}

	return map[string]interface{}{
		"method":      httpMethod,
		"mode":        "cors",
		"cache":       "default",
		"credentials": "include",
		"header": map[string]string{
			"Content-Type": "application/json; charset=UTF-8",
		},
		"body": string(jsonBodyString),
	}, nil
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

// --- Main Execution ---

func main() {
	log.Println("Chrome 브라우저를 실행하고 연결을 시도")

	// 1. Exec Allocator 옵션 설정
	// chromedp가 Chrome을 직접 실행하도록 설정합니다.
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),               // false로 설정하여 브라우저 창을 표시 (true로 바꾸면 백그라운드 실행)
		chromedp.Flag("disable-gpu", false),            // 일부 환경에서는 true가 필요할 수 있음
		chromedp.Flag("remote-debugging-port", "9222"), // 고정 포트 사용 (또는 0으로 설정하여 자동 선택)
		// 필요에 따라 사용자 데이터 디렉토리 지정 (로그인 유지 등에 유용)
		// chromedp.UserDataDir(filepath.Join(os.TempDir(), "chromedp-user-data")),
	)

	// 2. 새로운 Exec Allocator Context 생성
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc() // 프로그램 종료 시 할당자 컨텍스트 취소 (Chrome 프로세스 종료)

	// 3. 새로운 ChromeDP Context 생성 (Allocator 기반)
	// Allocator Context를 사용하여 새 탭을 위한 Context를 만듭니다.
	taskCtx, cancelTask := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf)) // 상세 로그 출력 설정
	defer cancelTask()                                                                  // 작업 컨텍스트 취소

	// Context 타임아웃 설정 (예: 60초)
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 60*time.Second)
	defer cancelTimeout()

	log.Println("Chrome 연결 성공")
	log.Println("자동으로 코드비머 페이지로 이동합니다, 30초안에 로그인을 진행해주세요.")

	GetTrackerHomePageTreeResult := ""
	err := chromedp.Run(taskCtx,
		// 대상 URL로 이동
		chromedp.Navigate(CodebeamerHost),
		chromedp.Sleep(30*time.Second),
		executeFetchInPage(
			fmt.Sprintf(GetTrackerHomePageTreeUrl, FcuProjectId),
			createFetchOption("POST", nil),
			&GetTrackerHomePageTreeResult,
		),
	)

	if err != nil {
		// 타임아웃 오류인지 확인
		if err == context.DeadlineExceeded {
			log.Fatalf("작업 시간 초과: %v", err)
		}
		log.Fatalf("ChromeDP 작업 실행 중 오류 발생: %v", err)
	}

	// 5. 결과 출력
	log.Printf("결과: %s", GetTrackerHomePageTreeResult)

	log.Println("작업 완료, Enter키를 누르면 이 프로그램과 Chrome이 종료됩니다.")
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
	log.Println(".")
}
