package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"coincheur/internal/detect"
	"coincheur/internal/frame"
	"coincheur/internal/scoring"
)

const (
	jpegQuality = 95
	folderMode  = 0o750
	fileMode    = 0o640
)

type Info struct {
	Time        time.Time         `json:"time"`
	Cards       []scoring.Card    `json:"cards"`
	Detections  []DetectionInfo   `json:"detections"`
	Settings    map[string]string `json:"settings"`
	FrameWidth  int               `json:"frameWidth"`
	FrameHeight int               `json:"frameHeight"`
	ImageFile   string            `json:"image"`
}

type DetectionInfo struct {
	Card  scoring.Card `json:"card"`
	Score float32      `json:"score"`
	X0    float32      `json:"x0"`
	Y0    float32      `json:"y0"`
	X1    float32      `json:"x1"`
	Y1    float32      `json:"y1"`
}

// Save writes the frame as a full resolution JPEG next to a JSON file describing what the app believed.
func Save(folder string, img frame.BGRA, detections []detect.Detection, cards []scoring.Card, settings map[string]string, now time.Time) (string, error) {
	if err := os.MkdirAll(folder, folderMode); err != nil {
		return "", fmt.Errorf("creating report folder: %w", err)
	}

	name := fmt.Sprintf("%s-%03d", now.Format("2006-01-02_15-04-05"), now.Nanosecond()/int(time.Millisecond))
	imagePath := filepath.Join(folder, name+".jpg")

	if err := writeImage(imagePath, img); err != nil {
		return "", err
	}

	info := Info{
		Time:        now,
		Cards:       cards,
		Detections:  describe(detections),
		Settings:    settings,
		FrameWidth:  img.Width,
		FrameHeight: img.Height,
		ImageFile:   filepath.Base(imagePath),
	}

	if err := writeInfo(filepath.Join(folder, name+".json"), info); err != nil {
		return "", errors.Join(err, os.Remove(imagePath))
	}

	return imagePath, nil
}

func writeImage(path string, img frame.BGRA) error {
	rgba := image.NewRGBA(image.Rect(0, 0, img.Width, img.Height))

	for row := range img.Height {
		for col := range img.Width {
			source := row*img.Stride + col*4
			target := rgba.PixOffset(col, row)

			rgba.Pix[target] = img.Pix[source+2]
			rgba.Pix[target+1] = img.Pix[source+1]
			rgba.Pix[target+2] = img.Pix[source]
			rgba.Pix[target+3] = 0xff
		}
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("creating report image: %w", err)
	}

	if err := jpeg.Encode(file, rgba, &jpeg.Options{Quality: jpegQuality}); err != nil {
		_ = file.Close()

		return fmt.Errorf("encoding report image: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("closing report image: %w", err)
	}

	return nil
}

func writeInfo(path string, info Info) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding report info: %w", err)
	}

	if err := os.WriteFile(path, data, fileMode); err != nil {
		return fmt.Errorf("writing report info: %w", err)
	}

	return nil
}

func describe(detections []detect.Detection) []DetectionInfo {
	infos := make([]DetectionInfo, 0, len(detections))

	for _, detection := range detections {
		infos = append(infos, DetectionInfo{
			Card:  detection.Card,
			Score: detection.Score,
			X0:    detection.Box.X0,
			Y0:    detection.Box.Y0,
			X1:    detection.Box.X1,
			Y1:    detection.Box.Y1,
		})
	}

	return infos
}
