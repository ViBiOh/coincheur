package scan

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"coincheur/internal/detect"
	"coincheur/internal/frame"
	"coincheur/internal/scoring"
)

func TestTrackerObserve(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		required   int
		frames     [][]scoring.Card
		wantAdded  [][]scoring.Card
		wantFinals []scoring.Card
	}{
		"confirmed on the second consecutive frame": {
			required:   2,
			frames:     [][]scoring.Card{{"AS"}, {"AS", "KD"}, {"AS", "KD"}},
			wantAdded:  [][]scoring.Card{nil, {"AS"}, {"KD"}},
			wantFinals: []scoring.Card{"AS", "KD"},
		},
		"a missed frame resets the streak": {
			required:   2,
			frames:     [][]scoring.Card{{"AS"}, {}, {"AS"}},
			wantAdded:  [][]scoring.Card{nil, nil, nil},
			wantFinals: nil,
		},
		"immediate confirmation": {
			required:   1,
			frames:     [][]scoring.Card{{"7C", "AS"}},
			wantAdded:  [][]scoring.Card{{"7C", "AS"}},
			wantFinals: []scoring.Card{"AS", "7C"},
		},
		"a confirmed card is not reported twice": {
			required:   1,
			frames:     [][]scoring.Card{{"AS"}, {"AS"}},
			wantAdded:  [][]scoring.Card{{"AS"}, nil},
			wantFinals: []scoring.Card{"AS"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tracker := NewTracker(test.required)

			for index, seen := range test.frames {
				assert.Equal(t, test.wantAdded[index], tracker.Observe(seen), "frame %d", index)
			}

			assert.Equal(t, test.wantFinals, tracker.Cards())
		})
	}
}

func TestTrackerToggleAndReset(t *testing.T) {
	t.Parallel()

	tracker := NewTracker(1)
	tracker.Observe([]scoring.Card{"AS"})

	tracker.Toggle("AS")
	assert.Empty(t, tracker.Observe([]scoring.Card{"AS"}), "a removed card stays removed")
	assert.Empty(t, tracker.Cards())

	tracker.Toggle("AS")
	assert.Equal(t, []scoring.Card{"AS"}, tracker.Cards(), "adding lifts the ban")

	tracker.Toggle("AS")
	tracker.Reset()
	assert.Equal(t, []scoring.Card{"KD"}, tracker.Observe([]scoring.Card{"KD"}))
	assert.Equal(t, []scoring.Card{"AS"}, tracker.Observe([]scoring.Card{"AS"}), "reset lifts every ban")
}

func TestEncodePreview(t *testing.T) {
	t.Parallel()

	const width, height = 8, 4

	img := frame.BGRA{Width: width, Height: height, Stride: width * 4, Pix: make([]byte, width*height*4)}
	for index := 0; index < len(img.Pix); index += 4 {
		img.Pix[index], img.Pix[index+1], img.Pix[index+2], img.Pix[index+3] = 0, 0, 255, 255
	}

	data, err := encodePreview(img, 4)
	require.NoError(t, err)

	decoded, err := jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)

	assert.Equal(t, 4, decoded.Bounds().Dx())
	assert.Equal(t, 2, decoded.Bounds().Dy())

	red, green, blue, _ := decoded.At(1, 1).RGBA()
	assert.Greater(t, red>>8, uint32(200), "red is kept")
	assert.Less(t, green>>8, uint32(60))
	assert.Less(t, blue>>8, uint32(60))
}

func TestEngineRun(t *testing.T) {
	t.Parallel()

	img := frame.BGRA{Width: 8, Height: 4, Stride: 32, Pix: make([]byte, 32*4)}
	cfg := Config{ConfirmScore: 0.5, PreviewWidth: 4, PreviewInterval: time.Hour}

	t.Run("reports a confirmed card at once and throttles the rest", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		detector := NewMockDetector(ctrl)
		detector.EXPECT().Detect(gomock.Any()).Return([]detect.Detection{
			{Card: "AS", Score: 0.9},
			{Card: "KD", Score: 0.3},
		}, nil).Times(3)

		frames := make(chan frame.BGRA, 3)
		for range 3 {
			frames <- img
		}

		close(frames)

		var updates []Update

		recycled := 0
		engine := NewEngine(detector, NewTracker(2), cfg)

		err := engine.Run(t.Context(), frames, func(frame.BGRA) { recycled++ }, func(update Update) {
			updates = append(updates, update)
		})

		require.NoError(t, err)

		require.Len(t, updates, 2, "the first frame is shown, the second confirms the card, the third is throttled")
		assert.Empty(t, updates[0].Added)
		assert.Equal(t, []scoring.Card{"AS"}, updates[1].Added)
		assert.NotEmpty(t, updates[0].Preview)
		assert.Equal(t, 3, recycled)
	})

	t.Run("wraps detector errors", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		detector := NewMockDetector(ctrl)
		detector.EXPECT().Detect(gomock.Any()).Return(nil, errors.New("boom"))

		frames := make(chan frame.BGRA, 1)
		frames <- img

		recycled := 0
		engine := NewEngine(detector, NewTracker(1), cfg)
		err := engine.Run(t.Context(), frames, func(frame.BGRA) { recycled++ }, func(Update) {})

		require.ErrorContains(t, err, "detecting cards: boom")
		assert.Equal(t, 1, recycled, "the failing frame is recycled once")
	})

	t.Run("recycles the kept frame too when a later detection fails", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		detector := NewMockDetector(ctrl)
		gomock.InOrder(
			detector.EXPECT().Detect(gomock.Any()).Return(nil, nil),
			detector.EXPECT().Detect(gomock.Any()).Return(nil, errors.New("boom")),
		)

		frames := make(chan frame.BGRA, 2)
		frames <- img

		frames <- img

		recycled := 0
		engine := NewEngine(detector, NewTracker(1), cfg)
		err := engine.Run(t.Context(), frames, func(frame.BGRA) { recycled++ }, func(Update) {})

		require.ErrorContains(t, err, "boom")
		assert.Equal(t, 2, recycled, "the failing frame and the kept one are recycled once each")
	})

	t.Run("stops when the context ends", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		engine := NewEngine(NewMockDetector(gomock.NewController(t)), NewTracker(1), cfg)

		require.NoError(t, engine.Run(ctx, make(chan frame.BGRA), func(frame.BGRA) {}, func(Update) {}))
	})
}

func TestEngineSnapshot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	detector := NewMockDetector(ctrl)
	detector.EXPECT().Detect(gomock.Any()).Return([]detect.Detection{{Card: "AS", Score: 0.9}}, nil).Times(2)

	pix := make([]byte, 32)
	pix[0] = 7

	frames := make(chan frame.BGRA, 2)
	frames <- frame.BGRA{Width: 8, Height: 1, Stride: 32, Pix: pix}
	frames <- frame.BGRA{Width: 8, Height: 1, Stride: 32, Pix: make([]byte, 32)}

	engine := NewEngine(detector, NewTracker(1), Config{ConfirmScore: 0.5, PreviewWidth: 4, PreviewInterval: time.Hour})

	_, found := engine.Snapshot()
	assert.False(t, found, "nothing before the first frame")

	var recycled int

	var snapshot Snapshot

	emit := func(Update) {
		if shot, ok := engine.Snapshot(); ok && shot.Frame.Pix[0] == 7 {
			snapshot = shot
		}
	}

	close(frames)

	// Recycled buffers are reused by the camera, so wipe them to prove the snapshot owns a copy.
	wipe := func(img frame.BGRA) {
		recycled++
		img.Pix[0] = 0
	}

	require.NoError(t, engine.Run(t.Context(), frames, wipe, emit))

	require.NotZero(t, snapshot.Frame.Width, "a snapshot was taken while the first frame was the last one")
	assert.Equal(t, byte(7), snapshot.Frame.Pix[0])
	assert.Equal(t, []scoring.Card{"AS"}, snapshot.Cards)
	require.Len(t, snapshot.Detections, 1)
	assert.Equal(t, 2, recycled, "both frames are recycled once the engine stops")

	_, found = engine.Snapshot()
	assert.False(t, found, "nothing is kept after the engine stops")
}
