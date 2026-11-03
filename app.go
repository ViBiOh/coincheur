package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"coincheur/internal/camera"
	"coincheur/internal/detect"
	"coincheur/internal/frame"
	"coincheur/internal/report"
	"coincheur/internal/scan"
	"coincheur/internal/scoring"
)

const (
	eventState = "state"
	eventFrame = "frame"

	reportFolderMode = 0o750

	confirmButton = "Effacer"
	cancelButton  = "Annuler"
)

type Scores struct {
	Scanned int `json:"scanned"`
	Other   int `json:"other"`
}

type State struct {
	Cards        []string `json:"cards"`
	Trump        string   `json:"trump"`
	LastTrick    string   `json:"lastTrick"`
	Belote       string   `json:"belote"`
	Scores       *Scores  `json:"scores"`
	CountWarning string   `json:"countWarning"`
	Scanning     bool     `json:"scanning"`
	Status       string   `json:"status"`
}

type BoxPayload struct {
	Card  string  `json:"card"`
	Score float32 `json:"score"`
	X0    float32 `json:"x0"`
	Y0    float32 `json:"y0"`
	X1    float32 `json:"x1"`
	Y1    float32 `json:"y1"`
}

type FramePayload struct {
	Preview     []byte       `json:"preview"`
	Boxes       []BoxPayload `json:"boxes"`
	InferenceMs int64        `json:"inferenceMs"`
}

type frameSource interface {
	Frames() <-chan frame.BGRA
	Recycle(img frame.BGRA)
	Stop()
}

type detectorHandle interface {
	scan.Detector
	Close() error
}

//go:generate go tool mockgen -destination=app_mock_test.go -package=main -source=app.go

type App struct {
	mutex   sync.Mutex
	emit    func(event string, payload any)
	tracker *scan.Tracker

	trump     scoring.Suit
	lastTrick scoring.Holder
	belote    scoring.Holder
	status    string

	detector detectorHandle
	engine   *scan.Engine
	cancel   context.CancelFunc
	finished chan struct{}
	scanning bool

	loadDetector func() (detectorHandle, error)
	startCamera  func() (frameSource, error)
	confirm      func(message string) bool
	saveReport   func(shot scan.Snapshot) (string, error)
	openFolder   func() error
}

func NewApp() *App {
	return &App{
		emit:       func(string, any) {},
		confirm:    func(string) bool { return true },
		saveReport: saveReport,
		openFolder: openReportFolder,
		tracker:    scan.NewTracker(confirmStreak),
		lastTrick:  scoring.Scanned,
		status:     "Montrez les cartes à la caméra.",
		loadDetector: func() (detectorHandle, error) {
			cfg, err := detectConfig()
			if err != nil {
				return nil, fmt.Errorf("locating model files: %w", err)
			}

			detector, err := detect.New(cfg)
			if err != nil {
				return nil, fmt.Errorf("loading model: %w", err)
			}

			return detector, nil
		},
		startCamera: func() (frameSource, error) {
			return camera.Start()
		},
	}
}

func (a *App) startup(ctx context.Context) {
	a.emit = func(event string, payload any) { runtime.EventsEmit(ctx, event, payload) }

	// The webview of macOS does not show window.confirm, so the question goes through a native dialog.
	a.confirm = func(message string) bool {
		choice, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:          runtime.QuestionDialog,
			Title:         "Nouvelle manche",
			Message:       message,
			Buttons:       []string{confirmButton, cancelButton},
			DefaultButton: cancelButton,
			CancelButton:  cancelButton,
		})

		return err == nil && choice == confirmButton
	}
}

func (a *App) shutdown(context.Context) {
	a.Stop()

	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.detector != nil {
		_ = a.detector.Close()
		a.detector = nil
	}
}

func (a *App) State() State {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	return a.stateLocked()
}

func (a *App) Deck() []string {
	deck := scoring.Deck()
	cards := make([]string, len(deck))

	for index, card := range deck {
		cards[index] = string(card)
	}

	return cards
}

// Start opens the camera and the model. The session is registered before the slow steps, so a Stop
// received while the user is answering the camera prompt cancels the start instead of being lost.
func (a *App) Start() error {
	a.mutex.Lock()

	if a.cancel != nil {
		a.mutex.Unlock()

		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	a.cancel, a.finished = cancel, finished
	a.status = "Démarrage de la caméra et du modèle..."
	a.emitStateLocked()
	a.mutex.Unlock()

	detector, source, err := a.prepare()

	a.mutex.Lock()
	defer a.mutex.Unlock()

	if err != nil {
		a.endSessionLocked(finished)

		return a.failLocked(err)
	}

	if ctx.Err() != nil {
		source.Stop()
		a.status = "Scan arrêté."
		a.endSessionLocked(finished)
		a.emitStateLocked()

		return nil
	}

	a.scanning = true
	a.status = "Scan en cours."

	go a.run(ctx, detector, source, finished)

	a.emitStateLocked()

	return nil
}

// prepare loads the model on first use and opens the camera, which may wait for the user to allow it.
func (a *App) prepare() (detectorHandle, frameSource, error) {
	a.mutex.Lock()
	detector := a.detector
	a.mutex.Unlock()

	if detector == nil {
		var err error

		detector, err = a.loadDetector()
		if err != nil {
			return nil, nil, err
		}

		a.mutex.Lock()
		a.detector = detector
		a.mutex.Unlock()
	}

	source, err := a.startCamera()
	if err != nil {
		return nil, nil, fmt.Errorf("starting camera: %w", err)
	}

	return detector, source, nil
}

func (a *App) Stop() {
	a.mutex.Lock()
	cancel, finished := a.cancel, a.finished
	a.mutex.Unlock()

	if cancel == nil {
		return
	}

	cancel()
	<-finished
}

func (a *App) endSessionLocked(finished chan<- struct{}) {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}

	close(finished)
}

// ReportError keeps the current camera image and what the app saw on it, to study the mistake later.
func (a *App) ReportError() error {
	a.mutex.Lock()
	engine := a.engine
	a.mutex.Unlock()

	if engine == nil {
		return a.reportFailure(errors.New("aucun scan en cours"))
	}

	shot, ok := engine.Snapshot()
	if !ok {
		return a.reportFailure(errors.New("aucune image reçue pour le moment"))
	}

	path, err := a.saveReport(shot)
	if err != nil {
		return a.reportFailure(err)
	}

	a.publish(func() { a.status = fmt.Sprintf("Image enregistrée pour analyse : %s", filepath.Base(path)) })

	return nil
}

func (a *App) OpenReports() error {
	if err := a.openFolder(); err != nil {
		return a.reportFailure(err)
	}

	return nil
}

func (a *App) reportFailure(err error) error {
	a.publish(func() { a.status = fmt.Sprintf("Erreur : %v", err) })

	return err
}

func saveReport(shot scan.Snapshot) (string, error) {
	folder, err := reportDir()
	if err != nil {
		return "", err
	}

	path, err := report.Save(folder, shot.Frame, shot.Detections, shot.Cards, reportSettings(), time.Now())
	if err != nil {
		return "", fmt.Errorf("saving report: %w", err)
	}

	return path, nil
}

func openReportFolder() error {
	folder, err := reportDir()
	if err != nil {
		return err
	}

	// The folder only exists after the first report, and open fails on a missing path.
	if err := os.MkdirAll(folder, reportFolderMode); err != nil {
		return fmt.Errorf("creating %s: %w", folder, err)
	}

	if err := exec.Command("open", folder).Run(); err != nil {
		return fmt.Errorf("opening %s: %w", folder, err)
	}

	return nil
}

func (a *App) ToggleCard(card string) error {
	parsed, err := scoring.ParseCard(card)
	if err != nil {
		return fmt.Errorf("toggling card: %w", err)
	}

	a.tracker.Toggle(parsed)
	a.publish(func() {})

	return nil
}

func (a *App) SetTrump(suit string) error {
	parsed, err := scoring.ParseSuit(suit)
	if err != nil {
		return fmt.Errorf("setting trump: %w", err)
	}

	a.publish(func() { a.trump = parsed })

	return nil
}

func (a *App) SetLastTrick(holder string) error {
	parsed, err := scoring.ParseHolder(holder)
	if err != nil {
		return fmt.Errorf("setting last trick: %w", err)
	}

	a.publish(func() { a.lastTrick = parsed })

	return nil
}

func (a *App) SetBelote(holder string) error {
	parsed, err := scoring.ParseHolder(holder)
	if err != nil {
		return fmt.Errorf("setting belote: %w", err)
	}

	a.publish(func() { a.belote = parsed })

	return nil
}

func (a *App) NewRound() {
	if !a.confirm("Effacer les cartes vues et les réglages de la manche ?") {
		return
	}

	a.tracker.Reset()
	a.publish(func() {
		a.trump = ""
		a.lastTrick = scoring.Scanned
		a.belote = scoring.NoHolder
	})
}

func (a *App) run(ctx context.Context, detector scan.Detector, source frameSource, finished chan<- struct{}) {
	engine := scan.NewEngine(detector, a.tracker, scanConfig())

	a.mutex.Lock()
	a.engine = engine
	a.mutex.Unlock()

	err := a.runEngine(ctx, engine, source)

	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.engine = nil
	a.scanning = false
	a.status = "Scan arrêté."

	if err != nil {
		a.status = fmt.Sprintf("Erreur : %v", err)
	}

	a.endSessionLocked(finished)
	a.emitStateLocked()
}

// runEngine owns the recovery of the scan goroutine: the model and the JPEG encoder must not take the app down.
func (a *App) runEngine(ctx context.Context, engine *scan.Engine, source frameSource) (err error) {
	defer source.Stop()

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("scan panicked: %v", recovered)
		}
	}()

	return engine.Run(ctx, source.Frames(), source.Recycle, a.onUpdate)
}

func (a *App) onUpdate(update scan.Update) {
	boxes := make([]BoxPayload, 0, len(update.Detections))

	for _, detection := range update.Detections {
		boxes = append(boxes, BoxPayload{
			Card:  string(detection.Card),
			Score: detection.Score,
			X0:    detection.Box.X0,
			Y0:    detection.Box.Y0,
			X1:    detection.Box.X1,
			Y1:    detection.Box.Y1,
		})
	}

	a.emit(eventFrame, FramePayload{Preview: update.Preview, Boxes: boxes, InferenceMs: update.Inference.Milliseconds()})

	if len(update.Added) > 0 {
		a.publish(func() {})
	}
}

func (a *App) publish(change func()) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	change()
	a.emitStateLocked()
}

func (a *App) failLocked(err error) error {
	a.status = fmt.Sprintf("Erreur : %v", err)
	a.emitStateLocked()

	return errors.New(a.status)
}

func (a *App) emitStateLocked() {
	a.emit(eventState, a.stateLocked())
}

func (a *App) stateLocked() State {
	cards := a.tracker.Cards()
	names := make([]string, len(cards))

	for index, card := range cards {
		names[index] = string(card)
	}

	state := State{
		Cards:     names,
		Trump:     string(a.trump),
		LastTrick: string(a.lastTrick),
		Belote:    string(a.belote),
		Scanning:  a.scanning,
		Status:    a.status,
	}

	if a.trump != "" {
		score := scoring.Compute(scoring.Input{Trump: a.trump, Cards: cards, LastTrick: a.lastTrick, Belote: a.belote})
		state.Scores = &Scores{Scanned: score.Scanned, Other: score.Other}
	}

	if !a.scanning {
		state.CountWarning = countWarning(len(cards))
	}

	return state
}

func countWarning(count int) string {
	extra, missing := scoring.CountIssue(count)
	if extra == 0 {
		return ""
	}

	plural := ""
	if missing > 1 {
		plural = "s"
	}

	return fmt.Sprintf("%d cartes lues, un multiple de 4 est attendu (%d manquante%s ou %d en trop).", count, missing, plural, extra)
}
