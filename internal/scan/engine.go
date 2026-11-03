package scan

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"coincheur/internal/detect"
	"coincheur/internal/frame"
	"coincheur/internal/scoring"
)

//go:generate go tool mockgen -destination=detector_mock_test.go -package=scan -source=engine.go -mock_names=Detector=MockDetector

type Detector interface {
	Detect(img frame.BGRA) ([]detect.Detection, error)
}

type Config struct {
	ConfirmScore    float32
	PreviewWidth    int
	PreviewInterval time.Duration
}

type Update struct {
	Preview    []byte
	Detections []detect.Detection
	Added      []scoring.Card
	Inference  time.Duration
}

// Snapshot is the last processed frame with what the engine made of it.
type Snapshot struct {
	Frame      frame.BGRA
	Detections []detect.Detection
	Cards      []scoring.Card
}

type Engine struct {
	detector Detector
	tracker  *Tracker
	cfg      Config

	mutex     sync.Mutex
	last      frame.BGRA
	lastFound []detect.Detection
	hasLast   bool
}

func NewEngine(detector Detector, tracker *Tracker, cfg Config) *Engine {
	return &Engine{detector: detector, tracker: tracker, cfg: cfg}
}

// Run processes frames until the context ends. The preview is throttled, but an update is
// always sent when a card was just confirmed so the interface reacts at once.
func (e *Engine) Run(ctx context.Context, frames <-chan frame.BGRA, recycle func(frame.BGRA), emit func(Update)) error {
	var lastPreview time.Time

	defer e.release(recycle)

	for {
		select {
		case <-ctx.Done():
			return nil
		case img, ok := <-frames:
			if !ok {
				return nil
			}

			update, err := e.process(img, recycle, &lastPreview)
			if err != nil {
				return err
			}

			if update != nil {
				emit(*update)
			}
		}
	}
}

// Snapshot copies the last processed frame, which stays out of the buffer pool until the next one replaces it.
func (e *Engine) Snapshot() (Snapshot, bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if !e.hasLast {
		return Snapshot{}, false
	}

	img := e.last
	img.Pix = slices.Clone(e.last.Pix)

	return Snapshot{
		Frame:      img,
		Detections: slices.Clone(e.lastFound),
		Cards:      e.tracker.Cards(),
	}, true
}

// process owns the frame until the engine keeps it, so a failing or panicking detector cannot leak the buffer.
func (e *Engine) process(img frame.BGRA, recycle func(frame.BGRA), lastPreview *time.Time) (*Update, error) {
	kept := false

	defer func() {
		if !kept {
			recycle(img)
		}
	}()

	start := time.Now()

	detections, err := e.detector.Detect(img)
	if err != nil {
		return nil, fmt.Errorf("detecting cards: %w", err)
	}

	inference := time.Since(start)

	e.keep(img, detections, recycle)

	kept = true

	added := e.tracker.Observe(e.confirmable(detections))
	if len(added) == 0 && time.Since(*lastPreview) < e.cfg.PreviewInterval {
		return nil, nil //nolint:nilnil // skipping an update is not an error
	}

	preview, err := encodePreview(img, e.cfg.PreviewWidth)
	if err != nil {
		return nil, err
	}

	*lastPreview = time.Now()

	return &Update{Preview: preview, Detections: detections, Added: added, Inference: inference}, nil
}

func (e *Engine) keep(img frame.BGRA, detections []detect.Detection, recycle func(frame.BGRA)) {
	e.mutex.Lock()
	previous, hadPrevious := e.last, e.hasLast
	e.last, e.lastFound, e.hasLast = img, detections, true
	e.mutex.Unlock()

	if hadPrevious {
		recycle(previous)
	}
}

func (e *Engine) release(recycle func(frame.BGRA)) {
	e.mutex.Lock()
	previous, hadPrevious := e.last, e.hasLast
	e.last, e.lastFound, e.hasLast = frame.BGRA{}, nil, false
	e.mutex.Unlock()

	if hadPrevious {
		recycle(previous)
	}
}

func (e *Engine) confirmable(detections []detect.Detection) []scoring.Card {
	var cards []scoring.Card

	for _, detection := range detections {
		if detection.Score >= e.cfg.ConfirmScore {
			cards = append(cards, detection.Card)
		}
	}

	return cards
}
