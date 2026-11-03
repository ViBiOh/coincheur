package main

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"coincheur/internal/detect"
	"coincheur/internal/frame"
	"coincheur/internal/scan"
	"coincheur/internal/scoring"
)

const eventually = 5 * time.Second

type lifecycle struct {
	app      *App
	source   *MockframeSource
	detector *MockdetectorHandle
	frames   chan frame.BGRA
	loads    atomic.Int32
	cameras  atomic.Int32
}

func newLifecycle(t *testing.T) *lifecycle {
	t.Helper()

	ctrl := gomock.NewController(t)
	env := &lifecycle{
		app:      NewApp(),
		source:   NewMockframeSource(ctrl),
		detector: NewMockdetectorHandle(ctrl),
		frames:   make(chan frame.BGRA, 1),
	}

	env.source.EXPECT().Frames().Return((<-chan frame.BGRA)(env.frames)).AnyTimes()
	env.source.EXPECT().Recycle(gomock.Any()).AnyTimes()

	env.app.loadDetector = func() (detectorHandle, error) {
		env.loads.Add(1)

		return env.detector, nil
	}

	env.app.startCamera = func() (frameSource, error) {
		env.cameras.Add(1)

		return env.source, nil
	}

	return env
}

func TestStartThenStop(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.source.EXPECT().Stop().Times(1)

	require.NoError(t, env.app.Start())
	assert.True(t, env.app.State().Scanning)

	env.app.Stop()

	state := env.app.State()

	assert.False(t, state.Scanning)
	assert.Equal(t, "Scan arrêté.", state.Status)
	assert.EqualValues(t, 1, env.loads.Load())
}

func TestStartTwiceOpensOneCamera(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.source.EXPECT().Stop().Times(1)

	require.NoError(t, env.app.Start())
	require.NoError(t, env.app.Start())

	env.app.Stop()

	assert.EqualValues(t, 1, env.cameras.Load())
}

func TestRestartAfterStop(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.source.EXPECT().Stop().Times(2)

	for range 2 {
		require.NoError(t, env.app.Start())
		env.app.Stop()
	}

	assert.EqualValues(t, 1, env.loads.Load(), "the model is loaded once")
	assert.EqualValues(t, 2, env.cameras.Load())
}

func TestStopDuringStartCancelsTheStart(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.source.EXPECT().Stop().Times(1)

	loading := make(chan struct{})
	release := make(chan struct{})

	env.app.loadDetector = func() (detectorHandle, error) {
		close(loading)
		<-release

		return env.detector, nil
	}

	started := make(chan error, 1)

	go func() { started <- env.app.Start() }()

	<-loading

	stopped := make(chan struct{})

	go func() {
		env.app.Stop()
		close(stopped)
	}()

	close(release)

	require.NoError(t, <-started)
	<-stopped

	assert.False(t, env.app.State().Scanning)
	assert.EqualValues(t, 1, env.cameras.Load(), "the camera that was opened is closed again")
}

func TestStartFailureKeepsTheAppUsable(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.app.startCamera = func() (frameSource, error) {
		return nil, errors.New("no camera")
	}

	require.ErrorContains(t, env.app.Start(), "starting camera: no camera")

	state := env.app.State()

	assert.False(t, state.Scanning)
	assert.Contains(t, state.Status, "no camera")

	env.app.Stop()
}

func TestDetectionErrorEndsTheScan(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.source.EXPECT().Stop().Times(1)
	env.detector.EXPECT().Detect(gomock.Any()).Return(nil, errors.New("boom"))

	require.NoError(t, env.app.Start())

	env.frames <- frame.BGRA{Width: 2, Height: 2, Stride: 8, Pix: make([]byte, 16)}

	require.Eventually(t, func() bool { return !env.app.State().Scanning }, eventually, 10*time.Millisecond)
	assert.Contains(t, env.app.State().Status, "boom")

	env.app.Stop()
}

func TestScanPanicIsContained(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.source.EXPECT().Stop().Times(1)
	env.detector.EXPECT().Detect(gomock.Any()).DoAndReturn(func(frame.BGRA) ([]detect.Detection, error) {
		panic("native failure")
	})

	require.NoError(t, env.app.Start())

	env.frames <- frame.BGRA{Width: 2, Height: 2, Stride: 8, Pix: make([]byte, 16)}

	require.Eventually(t, func() bool { return !env.app.State().Scanning }, eventually, 10*time.Millisecond)
	assert.Contains(t, env.app.State().Status, "native failure")

	env.app.Stop()
}

func TestShutdownWhileScanning(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.source.EXPECT().Stop().Times(1)
	env.detector.EXPECT().Close().Return(nil).Times(1)

	require.NoError(t, env.app.Start())

	env.app.shutdown(t.Context())

	assert.False(t, env.app.State().Scanning)
}

func TestReportErrorNeedsAScan(t *testing.T) {
	t.Parallel()

	app, _ := newTestApp()

	require.ErrorContains(t, app.ReportError(), "aucun scan en cours")
	assert.Contains(t, app.State().Status, "aucun scan en cours")
}

func TestReportErrorSavesTheCurrentFrame(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.source.EXPECT().Stop().Times(1)
	env.detector.EXPECT().Detect(gomock.Any()).Return([]detect.Detection{{Card: "AS", Score: 0.9}}, nil).AnyTimes()

	var saved scan.Snapshot

	env.app.saveReport = func(shot scan.Snapshot) (string, error) {
		saved = shot

		return "/somewhere/shot.jpg", nil
	}

	require.NoError(t, env.app.Start())

	pix := make([]byte, 16)
	pix[0] = 3
	env.frames <- frame.BGRA{Width: 2, Height: 2, Stride: 8, Pix: pix}

	require.Eventually(t, func() bool { return env.app.ReportError() == nil }, eventually, 10*time.Millisecond)

	status := env.app.State().Status

	env.app.Stop()

	assert.Equal(t, byte(3), saved.Frame.Pix[0])
	require.Len(t, saved.Detections, 1)
	assert.Equal(t, scoring.Card("AS"), saved.Detections[0].Card)
	assert.Contains(t, status, "shot.jpg")
}

func TestReportErrorSaveFailure(t *testing.T) {
	t.Parallel()

	env := newLifecycle(t)
	env.source.EXPECT().Stop().Times(1)
	env.detector.EXPECT().Detect(gomock.Any()).Return(nil, nil).AnyTimes()

	env.app.saveReport = func(scan.Snapshot) (string, error) {
		return "", errors.New("disk full")
	}

	require.NoError(t, env.app.Start())

	env.frames <- frame.BGRA{Width: 2, Height: 2, Stride: 8, Pix: make([]byte, 16)}

	require.Eventually(t, func() bool {
		err := env.app.ReportError()

		return err != nil && err.Error() == "disk full"
	}, eventually, 10*time.Millisecond)

	status := env.app.State().Status

	env.app.Stop()

	assert.Contains(t, status, "disk full")
}
