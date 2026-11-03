package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"coincheur/internal/frame"
	"coincheur/internal/scoring"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	// Rows are cx, cy, width, height, then one row per class, for 3 anchors.
	data := []float32{
		10, 20, 30,
		11, 21, 31,
		40, 41, 42,
		50, 51, 52,
		0.9, 0.1, 0.2,
		0.1, 0.8, 0.3,
	}

	tests := map[string]struct {
		minScore float32
		want     []candidate
	}{
		"keeps the best class above the threshold": {
			minScore: 0.25,
			want: []candidate{
				{class: 0, score: 0.9, centerX: 10, centerY: 11, width: 40, height: 50},
				{class: 1, score: 0.8, centerX: 20, centerY: 21, width: 41, height: 51},
				{class: 1, score: 0.3, centerX: 30, centerY: 31, width: 42, height: 52},
			},
		},
		"drops weak anchors": {
			minScore: 0.5,
			want: []candidate{
				{class: 0, score: 0.9, centerX: 10, centerY: 11, width: 40, height: 50},
				{class: 1, score: 0.8, centerX: 20, centerY: 21, width: 41, height: 51},
			},
		},
		"nothing above the threshold": {
			minScore: 0.95,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := decode(data, 3, 2, test.minScore)

			require.Len(t, got, len(test.want))

			for index, want := range test.want {
				assert.Equal(t, want.class, got[index].class)
				assert.InDelta(t, want.score, got[index].score, 1e-6)
				assert.InDelta(t, want.centerX, got[index].centerX, 1e-6)
				assert.InDelta(t, want.height, got[index].height, 1e-6)
			}
		})
	}
}

func TestSuppress(t *testing.T) {
	t.Parallel()

	box := func(class int, score, centerX float32) candidate {
		return candidate{class: class, score: score, centerX: centerX, centerY: 50, width: 40, height: 60}
	}

	tests := map[string]struct {
		input []candidate
		want  []int
	}{
		"same class overlapping keeps the best": {
			input: []candidate{box(1, 0.4, 50), box(1, 0.9, 52)},
			want:  []int{1},
		},
		"other classes overlapping are kept": {
			input: []candidate{box(1, 0.4, 50), box(2, 0.9, 52)},
			want:  []int{2, 1},
		},
		"same class far apart is kept twice": {
			input: []candidate{box(3, 0.7, 50), box(3, 0.6, 400)},
			want:  []int{3, 3},
		},
		"nothing": {},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got []int
			for _, kept := range suppress(test.input) {
				got = append(got, kept.class)
			}

			assert.Equal(t, test.want, got)
		})
	}
}

func TestOverlap(t *testing.T) {
	t.Parallel()

	same := candidate{centerX: 50, centerY: 50, width: 20, height: 20}
	apart := candidate{centerX: 500, centerY: 50, width: 20, height: 20}
	half := candidate{centerX: 60, centerY: 50, width: 20, height: 20}

	assert.InDelta(t, 1, overlap(same, same), 1e-6)
	assert.InDelta(t, 0, overlap(same, apart), 1e-6)
	assert.InDelta(t, 1.0/3, overlap(same, half), 1e-6)
}

func TestLetterboxFill(t *testing.T) {
	t.Parallel()

	const size = 8

	img := frame.BGRA{Width: 4, Height: 2, Stride: 16, Pix: make([]byte, 16*2)}
	for index := 0; index < len(img.Pix); index += 4 {
		img.Pix[index], img.Pix[index+1], img.Pix[index+2], img.Pix[index+3] = 0, 128, 255, 255
	}

	fit := newLetterbox(img.Width, img.Height, size)
	tensor := make([]float32, 3*size*size)
	fit.fill(img, tensor)

	plane := size * size

	assert.InDelta(t, 2, fit.scale, 1e-6)
	assert.InDelta(t, 2, fit.offset[1], 1e-6)

	center := 4*size + 4

	assert.InDelta(t, 1, tensor[center], 1e-6, "red")
	assert.InDelta(t, 128.0/255, tensor[plane+center], 1e-6, "green")
	assert.InDelta(t, 0, tensor[2*plane+center], 1e-6, "blue")
	assert.InDelta(t, padValue, tensor[0], 1e-6, "top padding")
	assert.InDelta(t, padValue, tensor[plane-1], 1e-6, "bottom padding")
}

func TestDetectionsMapBackToTheFrame(t *testing.T) {
	t.Parallel()

	fit := newLetterbox(1280, 720, 640)
	deck := scoring.Deck()

	got := fit.detections([]candidate{
		{class: 8, score: 0.8, centerX: 320, centerY: 320, width: 64, height: 64},
		{class: 0, score: 0.7, centerX: 5, centerY: 5, width: 40, height: 40},
	}, deck, 1280, 720)

	require.Len(t, got, 2)

	assert.Equal(t, scoring.Card("AH"), got[0].Card)
	assert.InDelta(t, 0.45, got[0].Box.X0, 1e-6)
	assert.InDelta(t, 0.55, got[0].Box.X1, 1e-6)
	assert.InDelta(t, (360.0-64)/720, got[0].Box.Y0, 1e-6)
	assert.InDelta(t, 0, got[1].Box.Y0, 1e-6, "clamped to the frame")
}

func TestFirstExisting(t *testing.T) {
	t.Parallel()

	folder := t.TempDir()
	present := filepath.Join(folder, "present.bin")
	require.NoError(t, os.WriteFile(present, []byte("x"), 0o600))

	got, err := firstExisting([]string{filepath.Join(folder, "missing.bin"), folder, present})
	require.NoError(t, err)
	assert.Equal(t, present, got)

	_, err = firstExisting([]string{filepath.Join(folder, "missing.bin")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing.bin")
}
