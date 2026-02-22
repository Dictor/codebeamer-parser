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
	formatter logrus.Formatter
	logData   binding.String
	progData  binding.Float
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

	b, _ := h.formatter.Format(entry)
	msg := string(b)

	curr, _ := h.logData.Get()
	lines := strings.Split(curr, "\n")
	// 로그가 너무 길어지면 성능 저하되므로 최근 200줄 유지
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	newLog := strings.Join(lines, "\n") + msg
	h.logData.Set(newLog)

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
	logData.Set("GUI Loaded. Ready to run.\n")

	progData := binding.NewFloat()

	logEntry := widget.NewEntryWithData(logData)
	logEntry.MultiLine = true

	progressBar := widget.NewProgressBarWithData(progData)

	Logger.AddHook(&guiLogHook{
		formatter: &logrus.TextFormatter{DisableColors: true},
		logData:   logData,
		progData:  progData,
	})

	var runBtn *widget.Button
	runBtn = widget.NewButton("Run Parser", func() {
		runBtn.Disable()
		go func() {
			defer runBtn.Enable()

			d := debugCheck.Checked
			g := graphCheck.Checked
			s := skipCheck.Checked
			p := partialEntry.Text

			logData.Set("Starting parser...\n")
			progData.Set(0)

			runLogic(d, g, s, p)

			curr, _ := logData.Get()
			logData.Set(curr + "\nDone.\n")
		}()
	})

	form := container.NewVBox(
		widget.NewLabel("CLI Execution Options"),
		debugCheck,
		graphCheck,
		skipCheck,
		container.NewBorder(nil, nil, widget.NewLabel("Partial Crawl ID: "), nil, partialEntry),
		widget.NewLabel(""),
		progressBar,
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
