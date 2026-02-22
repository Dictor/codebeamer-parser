package main

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/sirupsen/logrus"
)

type guiLogHook struct {
	formatter    logrus.Formatter
	logData      binding.String
	progData     binding.Float
	etaData      binding.String
	stepData     binding.String
	onLogUpdated func()
}

func (h *guiLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *guiLogHook) Fire(entry *logrus.Entry) error {
	// 진행률(progress) 파싱 (예: "50.00%")
	if progStr, ok := entry.Data["progress"].(string); ok {
		progStr = strings.TrimSuffix(progStr, "%")
		if p, err := strconv.ParseFloat(progStr, 64); err == nil {
			h.progData.Set(p / 100.0)
		}
	}

	// ETA 파싱
	if etaStr, ok := entry.Data["eta"].(string); ok {
		// Fyne UI에서 ETA를 표시할 바인딩이 있다면 거기에 업데이트
		if h.etaData != nil {
			h.etaData.Set("ETA: " + etaStr)
		}
	}

	// 현재 작업 단계(step) 파싱 (예: "2) 루트 및 차일드 트래커 크롤링")
	if stepStr, ok := entry.Data["stepName"].(string); ok {
		if h.stepData != nil {
			h.stepData.Set("Current Step: " + stepStr)
		}
	}

	// 시간 및 레벨을 포함하여 가독성 좋게 포맷팅
	msg := entry.Time.Format("15:04:05") + " [" + strings.ToUpper(entry.Level.String()) + "] " + entry.Message

	curr, _ := h.logData.Get()
	lines := strings.Split(curr, "\n")
	// 로그가 너무 길어지면 성능 저하되므로 최근 200줄 유지
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	newLog := strings.Join(lines, "\n")
	if newLog != "" {
		newLog += "\n"
	}
	newLog += msg
	h.logData.Set(newLog)

	// Fyne widget에서 자동으로 스크롤 맨 아래로 내리기 위한 콜백
	if h.onLogUpdated != nil {
		h.onLogUpdated()
	}

	return nil
}

func startGUI(debugLog, saveGraph, skipCrawling bool, partialCrawling string) {
	a := app.New()
	w := a.NewWindow("Codebeamer Parser GUI")
	w.Resize(fyne.NewSize(800, 600))

	debugCheck := widget.NewCheck("Debug Log", nil)
	debugCheck.SetChecked(debugLog)

	graphCheck := widget.NewCheck("Save Graph Image", nil)
	graphCheck.SetChecked(saveGraph)

	skipCheck := widget.NewCheck("Skip Crawling", nil)
	skipCheck.SetChecked(skipCrawling)

	partialEntry := widget.NewEntry()
	partialEntry.SetText(partialCrawling)
	partialEntry.SetPlaceHolder("Tracker ID (leave empty for full crawl)")

	logData := binding.NewString()
	logData.Set("GUI Loaded. Ready to run.")

	progData := binding.NewFloat()
	etaData := binding.NewString()
	etaData.Set("ETA: -")
	stepData := binding.NewString()
	stepData.Set("Current Step: Ready")

	logEntry := widget.NewEntryWithData(logData)
	logEntry.MultiLine = true
	// Entry natively handles scrolling and tends to be safer with large text updates
	logEntry.Disable() // Make it read-only

	progressBar := widget.NewProgressBarWithData(progData)
	etaLabel := widget.NewLabelWithData(etaData)
	stepLabel := widget.NewLabelWithData(stepData)
	stepLabel.TextStyle = fyne.TextStyle{Bold: true}

	Logger.AddHook(&guiLogHook{
		formatter:    &logrus.TextFormatter{DisableColors: true},
		logData:      logData,
		progData:     progData,
		etaData:      etaData,
		stepData:     stepData,
		onLogUpdated: nil, // Note: ScrollToBottom() is not thread-safe.
	})

	var runBtn *widget.Button
	runBtn = widget.NewButton("Run Parser", func() {
		runBtn.Disable()
		go func() {
			d := debugCheck.Checked
			g := graphCheck.Checked
			s := skipCheck.Checked
			p := partialEntry.Text

			logData.Set("Starting parser...")
			progData.Set(0)
			stepData.Set("Current Step: (1/5) pre-process for crawling")

			runLogic(d, g, s, p)

			curr, _ := logData.Get()
			logData.Set(curr + "\nDone.")
			stepData.Set("Current Step: Finished")
			etaData.Set("ETA: 0s")
			progData.Set(1.0)
		}()
	})

	form := container.NewVBox(
		widget.NewLabel("CLI Execution Options"),
		debugCheck,
		graphCheck,
		skipCheck,
		container.NewBorder(nil, nil, widget.NewLabel("Partial Crawl ID: "), nil, partialEntry),
		widget.NewLabel(""),
		stepLabel,
		container.NewBorder(nil, nil, nil, etaLabel, progressBar),
		runBtn,
	)

	split := container.NewVSplit(
		form,
		logEntry,
	)
	split.SetOffset(0.3)

	w.SetContent(split)
	w.ShowAndRun()
}
