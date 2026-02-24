package main

import (
	"os"
	"strconv"
	"strings"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/sirupsen/logrus"
)

type guiLogHook struct {
	formatter logrus.Formatter
	window    *app.Window
	state     *guiState
}

func (h *guiLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *guiLogHook) Fire(entry *logrus.Entry) error {
	// 진행률(progress) 파싱 (예: "50.00%")
	if progStr, ok := entry.Data["progress"].(string); ok {
		progStr = strings.TrimSuffix(progStr, "%")
		if p, err := strconv.ParseFloat(progStr, 64); err == nil {
			h.state.progress = float32(p / 100.0)
		}
	}

	// ETA 파싱
	if etaStr, ok := entry.Data["eta"].(string); ok {
		h.state.etaText = "ETA: " + etaStr
	}

	// 현재 작업 단계(step) 파싱
	if stepStr, ok := entry.Data["stepName"].(string); ok {
		h.state.stepText = "Current Step: " + stepStr
	}

	msg := entry.Time.Format("15:04:05") + " [" + strings.ToUpper(entry.Level.String()) + "] " + entry.Message

	h.state.logs = append(h.state.logs, msg)
	if len(h.state.logs) > 500 {
		h.state.logs = h.state.logs[len(h.state.logs)-500:]
	}

	h.window.Invalidate()
	return nil
}

type guiState struct {
	debugLog        widget.Bool
	saveGraphSvg    widget.Bool
	saveGraphHtml   widget.Bool
	skipCrawling    widget.Bool
	partialCrawling widget.Editor
	runBtn          widget.Clickable
	logsList        widget.List

	logs     []string
	progress float32
	etaText  string
	stepText string

	isRunning bool
}

func startGUI(debugLog, saveGraphSvg, saveGraphHtml, skipCrawling bool, partialCrawling string, guiMode bool) {
	state := &guiState{
		etaText:  "ETA: -",
		stepText: "Current Step: Ready",
		logs:     []string{"GUI Loaded. Ready to run."},
	}
	state.debugLog.Value = debugLog
	state.saveGraphSvg.Value = saveGraphSvg
	state.saveGraphHtml.Value = saveGraphHtml
	state.skipCrawling.Value = skipCrawling
	state.partialCrawling.SetText(partialCrawling)
	state.partialCrawling.SingleLine = true

	state.logsList.Axis = layout.Vertical

	go func() {
		w := new(app.Window)
		w.Option(app.Title("Codebeamer Parser GUI"), app.Size(unit.Dp(800), unit.Dp(600)))

		Logger.AddHook(&guiLogHook{
			formatter: &logrus.TextFormatter{DisableColors: true},
			window:    w,
			state:     state,
		})

		if err := loop(w, state, guiMode); err != nil {
			logrus.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func loop(w *app.Window, state *guiState, guiMode bool) error {
	th := material.NewTheme()

	// To make sure logs auto-scroll when new items arrive
	lastLogCount := 0

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			if state.runBtn.Clicked(gtx) && !state.isRunning {
				state.isRunning = true

				d := state.debugLog.Value
				gSvg := state.saveGraphSvg.Value
				gHtml := state.saveGraphHtml.Value
				s := state.skipCrawling.Value
				p := state.partialCrawling.Text()

				state.logs = append(state.logs, "Starting parser...")
				state.progress = 0
				state.stepText = "Current Step: (1/5) pre-process for crawling"

				go func() {
					runLogic(d, gSvg, gHtml, s, p, guiMode)
					state.logs = append(state.logs, "Done.")
					state.stepText = "Current Step: Finished"
					state.etaText = "ETA: 0s"
					state.progress = 1.0
					state.isRunning = false
					w.Invalidate()
				}()
			}

			// check log scroll
			if len(state.logs) > lastLogCount {
				state.logsList.ScrollToEnd = true
				lastLogCount = len(state.logs)
			}

			layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								title := material.H6(th, "CLI Execution Options")
								return title.Layout(gtx)
							}),
							layout.Rigid(material.CheckBox(th, &state.debugLog, "Debug Log").Layout),
							layout.Rigid(material.CheckBox(th, &state.saveGraphSvg, "Save Graph SVG").Layout),
							layout.Rigid(material.CheckBox(th, &state.saveGraphHtml, "Save Graph HTML").Layout),
							layout.Rigid(material.CheckBox(th, &state.skipCrawling, "Skip Crawling").Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(material.Body1(th, "Partial Crawl ID: ").Layout),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										ed := material.Editor(th, &state.partialCrawling, "Tracker ID (leave empty for full crawl)")
										return ed.Layout(gtx)
									}),
								)
							}),
						)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body1(th, state.stepText)
								lbl.Font.Weight = font.Bold
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(material.Body1(th, state.etaText).Layout),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Flexed(1, material.ProgressBar(th, state.progress).Layout),
								)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := material.Button(th, &state.runBtn, "Run Parser")
								if state.isRunning {
									gtx = gtx.Disabled()
								}
								return btn.Layout(gtx)
							}),
						)
					})
				}),
				// Log view
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						// draw background for log
						// ops.Fill(...) could be used but let's keep it simple
						return material.List(th, &state.logsList).Layout(gtx, len(state.logs), func(gtx layout.Context, index int) layout.Dimensions {
							lbl := material.Body2(th, state.logs[index])
							return lbl.Layout(gtx)
						})
					})
				}),
			)
			e.Frame(gtx.Ops)
		}
	}
}
