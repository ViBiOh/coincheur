package report

import (
	"encoding/json"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"coincheur/internal/detect"
	"coincheur/internal/frame"
	"coincheur/internal/scoring"
)

func TestSave(t *testing.T) {
	t.Parallel()

	folder := filepath.Join(t.TempDir(), "reports")
	now := time.Date(2026, 11, 3, 18, 30, 5, 42*int(time.Millisecond), time.UTC)

	img := frame.BGRA{Width: 4, Height: 2, Stride: 16, Pix: make([]byte, 32)}
	for index := 0; index < len(img.Pix); index += 4 {
		img.Pix[index], img.Pix[index+1], img.Pix[index+2], img.Pix[index+3] = 0, 0, 255, 255
	}

	detections := []detect.Detection{{Card: "AS", Score: 0.9, Box: detect.Box{X0: 0.1, Y0: 0.2, X1: 0.3, Y1: 0.4}}}

	path, err := Save(folder, img, detections, []scoring.Card{"AS", "KD"}, map[string]string{"confirmScore": "0.55"}, now)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(folder, "2026-11-03_18-30-05-042.jpg"), path)

	file, err := os.Open(path)
	require.NoError(t, err)

	defer func() { _ = file.Close() }()

	decoded, err := jpeg.Decode(file)
	require.NoError(t, err)

	red, green, blue, _ := decoded.At(1, 1).RGBA()

	assert.Equal(t, 4, decoded.Bounds().Dx())
	assert.Greater(t, red>>8, uint32(200), "red is kept")
	assert.Less(t, green>>8, uint32(40))
	assert.Less(t, blue>>8, uint32(40))

	data, err := os.ReadFile(filepath.Join(folder, "2026-11-03_18-30-05-042.json"))
	require.NoError(t, err)

	var info Info

	require.NoError(t, json.Unmarshal(data, &info))

	assert.Equal(t, []scoring.Card{"AS", "KD"}, info.Cards)
	assert.Equal(t, "2026-11-03_18-30-05-042.jpg", info.ImageFile)
	assert.Equal(t, "0.55", info.Settings["confirmScore"])
	require.Len(t, info.Detections, 1)
	assert.InDelta(t, 0.9, info.Detections[0].Score, 1e-6)
}

func TestSaveDoesNotOverwrite(t *testing.T) {
	t.Parallel()

	folder := t.TempDir()
	now := time.Date(2026, 11, 3, 18, 30, 5, 0, time.UTC)
	img := frame.BGRA{Width: 2, Height: 2, Stride: 8, Pix: make([]byte, 16)}

	_, err := Save(folder, img, nil, nil, nil, now)
	require.NoError(t, err)

	_, err = Save(folder, img, nil, nil, nil, now)
	require.ErrorContains(t, err, "creating report image")
}

func TestSaveRemovesTheImageWhenTheInfoFails(t *testing.T) {
	t.Parallel()

	folder := t.TempDir()
	now := time.Date(2026, 11, 3, 18, 30, 5, 0, time.UTC)
	img := frame.BGRA{Width: 2, Height: 2, Stride: 8, Pix: make([]byte, 16)}

	// A folder where the info file should go makes writing it fail.
	require.NoError(t, os.Mkdir(filepath.Join(folder, "2026-11-03_18-30-05-000.json"), 0o750))

	_, err := Save(folder, img, nil, nil, nil, now)

	require.ErrorContains(t, err, "writing report info")
	assert.NoFileExists(t, filepath.Join(folder, "2026-11-03_18-30-05-000.jpg"))
}
