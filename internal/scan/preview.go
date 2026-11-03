package scan

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"coincheur/internal/frame"
)

const previewQuality = 70

// encodePreview downscales the frame by sampling and returns it as a JPEG.
func encodePreview(img frame.BGRA, width int) ([]byte, error) {
	width = min(width, img.Width)
	height := img.Height * width / img.Width

	preview := image.NewRGBA(image.Rect(0, 0, width, height))

	for row := range height {
		sourceRow := row * img.Height / height

		for col := range width {
			source := sourceRow*img.Stride + (col*img.Width/width)*4
			target := preview.PixOffset(col, row)

			preview.Pix[target] = img.Pix[source+2]
			preview.Pix[target+1] = img.Pix[source+1]
			preview.Pix[target+2] = img.Pix[source]
			preview.Pix[target+3] = 0xff
		}
	}

	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, preview, &jpeg.Options{Quality: previewQuality}); err != nil {
		return nil, fmt.Errorf("encoding preview: %w", err)
	}

	return buffer.Bytes(), nil
}
