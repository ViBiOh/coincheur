package main

import (
	"errors"
	"flag"
	"fmt"
	"image/jpeg"
	"os"
	"slices"
	"strings"
	"time"

	"coincheur/internal/detect"
	"coincheur/internal/frame"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	provider := flag.String("provider", detect.DefaultProvider(), "coreml or cpu")
	units := flag.String("units", "ALL", "coreml compute units")
	minScore := flag.Float64("min", 0.25, "minimum confidence")
	runs := flag.Int("runs", 5, "warm runs used for the timing")
	flag.Parse()

	if flag.NArg() == 0 {
		return errors.New("usage: evaluate [flags] photo.jpg")
	}

	library, err := detect.LocateLibrary()
	if err != nil {
		return fmt.Errorf("locating onnxruntime: %w", err)
	}

	model, err := detect.LocateModel()
	if err != nil {
		return fmt.Errorf("locating model: %w", err)
	}

	detector, err := detect.New(detect.Config{
		LibraryPath:  library,
		ModelPath:    model,
		Provider:     *provider,
		ComputeUnits: *units,
		MinScore:     float32(*minScore),
	})
	if err != nil {
		return fmt.Errorf("creating detector: %w", err)
	}

	defer func() { _ = detector.Close() }()

	for _, path := range flag.Args() {
		if err := evaluate(detector, path, max(1, *runs)); err != nil {
			return err
		}
	}

	return nil
}

func evaluate(detector *detect.Detector, path string, runs int) error {
	img, err := load(path)
	if err != nil {
		return err
	}

	detections, err := detector.Detect(img)
	if err != nil {
		return err
	}

	start := time.Now()

	for range runs {
		if _, err := detector.Detect(img); err != nil {
			return err
		}
	}

	best := make(map[string]float32)

	for _, detection := range detections {
		card := string(detection.Card)
		best[card] = max(best[card], detection.Score)
	}

	cards := make([]string, 0, len(best))
	for card := range best {
		cards = append(cards, card)
	}

	slices.SortFunc(cards, func(a, b string) int { return int(best[b]*1000) - int(best[a]*1000) })

	parts := make([]string, 0, len(cards))
	for _, card := range cards {
		parts = append(parts, fmt.Sprintf("%s %.2f", card, best[card]))
	}

	fmt.Printf("%s: %d cards in %s per frame\n  %s\n", path, len(cards), (time.Since(start) / time.Duration(runs)).Round(time.Millisecond), strings.Join(parts, ", "))

	return nil
}

func load(path string) (frame.BGRA, error) {
	file, err := os.Open(path)
	if err != nil {
		return frame.BGRA{}, fmt.Errorf("opening photo: %w", err)
	}

	defer func() { _ = file.Close() }()

	decoded, err := jpeg.Decode(file)
	if err != nil {
		return frame.BGRA{}, fmt.Errorf("decoding %s: %w", path, err)
	}

	bounds := decoded.Bounds()
	img := frame.BGRA{Width: bounds.Dx(), Height: bounds.Dy(), Stride: bounds.Dx() * 4}
	img.Pix = make([]byte, img.Stride*img.Height)

	for row := range img.Height {
		for col := range img.Width {
			red, green, blue, _ := decoded.At(bounds.Min.X+col, bounds.Min.Y+row).RGBA()
			offset := row*img.Stride + col*4
			img.Pix[offset], img.Pix[offset+1], img.Pix[offset+2], img.Pix[offset+3] = uint8(blue>>8), uint8(green>>8), uint8(red>>8), 255
		}
	}

	return img, nil
}
