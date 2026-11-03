package detect

import (
	"runtime"
	"sync"

	"coincheur/internal/frame"
)

const padValue = float32(114) / 255

// letterbox keeps how a frame was fitted in the square model input, to map boxes back.
type letterbox struct {
	size   int
	scale  float32
	offset [2]float32
}

func newLetterbox(width, height, size int) letterbox {
	scale := min(float32(size)/float32(width), float32(size)/float32(height))

	return letterbox{
		size:  size,
		scale: scale,
		offset: [2]float32{
			(float32(size) - float32(width)*scale) / 2,
			(float32(size) - float32(height)*scale) / 2,
		},
	}
}

// fill writes the frame as normalized planar RGB, resized with bilinear sampling and padded with gray.
func (l letterbox) fill(img frame.BGRA, tensor []float32) {
	plane := l.size * l.size

	for index := range tensor {
		tensor[index] = padValue
	}

	left, top := int(l.offset[0]+0.5), int(l.offset[1]+0.5)
	drawWidth := min(int(float32(img.Width)*l.scale+0.5), l.size-left)
	drawHeight := min(int(float32(img.Height)*l.scale+0.5), l.size-top)

	columns := make([]sampleAxis, drawWidth)
	for col := range columns {
		columns[col] = newSampleAxis(col, l.scale, img.Width)
	}

	workers := max(1, runtime.GOMAXPROCS(0))
	rowsPerWorker := (drawHeight + workers - 1) / workers

	var group sync.WaitGroup

	for worker := range workers {
		first := worker * rowsPerWorker
		last := min(first+rowsPerWorker, drawHeight)

		group.Go(func() {
			for row := first; row < last; row++ {
				l.fillRow(img, tensor, columns, newSampleAxis(row, l.scale, img.Height), (top+row)*l.size+left, plane)
			}
		})
	}

	group.Wait()
}

func (l letterbox) fillRow(img frame.BGRA, tensor []float32, columns []sampleAxis, rowAxis sampleAxis, target, plane int) {
	topLine := rowAxis.first * img.Stride
	bottomLine := rowAxis.second * img.Stride

	for col, colAxis := range columns {
		leftOffset, rightOffset := colAxis.first*4, colAxis.second*4

		var channels [3]float32

		for channel := range channels {
			// BGRA stores blue first, while the model expects red first.
			source := 2 - channel

			topLeft := float32(img.Pix[topLine+leftOffset+source])
			topRight := float32(img.Pix[topLine+rightOffset+source])
			bottomLeft := float32(img.Pix[bottomLine+leftOffset+source])
			bottomRight := float32(img.Pix[bottomLine+rightOffset+source])

			top := topLeft + (topRight-topLeft)*colAxis.fraction
			bottom := bottomLeft + (bottomRight-bottomLeft)*colAxis.fraction
			channels[channel] = (top + (bottom-top)*rowAxis.fraction) / 255
		}

		tensor[target+col] = channels[0]
		tensor[plane+target+col] = channels[1]
		tensor[2*plane+target+col] = channels[2]
	}
}

// sampleAxis holds the two source pixels, and the blend between them, for one output coordinate.
type sampleAxis struct {
	first    int
	second   int
	fraction float32
}

func newSampleAxis(position int, scale float32, limit int) sampleAxis {
	source := min(max((float32(position)+0.5)/scale-0.5, 0), float32(limit-1))
	first := int(source)

	return sampleAxis{first: first, second: min(first+1, limit-1), fraction: source - float32(first)}
}

func (l letterbox) unmap(x, y float32) (float32, float32) {
	return (x - l.offset[0]) / l.scale, (y - l.offset[1]) / l.scale
}
