package camera

/*
#cgo CFLAGS: -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework AVFoundation -framework CoreMedia -framework CoreVideo -framework Foundation
#include <stdlib.h>
#include "camera_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime/cgo"
	"sync"
	"unsafe"

	"coincheur/internal/frame"
)

var errAlreadyRunning = errors.New("camera already running")

type Camera struct {
	frames chan frame.BGRA
	pool   sync.Pool
	handle cgo.Handle
	once   sync.Once
}

// Start opens the default camera, asking the user for access the first time.
func Start() (*Camera, error) {
	cam := &Camera{frames: make(chan frame.BGRA, 1)}
	cam.handle = cgo.NewHandle(cam)

	var message *C.char
	if code := C.cameraStart(C.uintptr_t(cam.handle), &message); code != 0 {
		defer C.free(unsafe.Pointer(message))
		cam.handle.Delete()

		if code == 1 {
			return nil, errAlreadyRunning
		}

		return nil, fmt.Errorf("starting camera: %s", C.GoString(message))
	}

	return cam, nil
}

// Frames delivers the most recent frame only: a frame not consumed in time is replaced.
func (c *Camera) Frames() <-chan frame.BGRA {
	return c.frames
}

// Recycle hands the buffer of a processed frame back for reuse.
func (c *Camera) Recycle(img frame.BGRA) {
	c.pool.Put(img.Pix) //nolint:staticcheck // slices are stored as is, the allocation of the header is negligible
}

func (c *Camera) Stop() {
	c.once.Do(func() {
		C.cameraStop()
		c.handle.Delete()
	})
}

func (c *Camera) push(img frame.BGRA) {
	for {
		select {
		case c.frames <- img:
			return
		default:
			select {
			case stale := <-c.frames:
				c.Recycle(stale)
			default:
			}
		}
	}
}

func (c *Camera) buffer(size int) []byte {
	if pix, ok := c.pool.Get().([]byte); ok && cap(pix) >= size {
		return pix[:size]
	}

	return make([]byte, size)
}

//export cameraFrame
func cameraFrame(handle C.uintptr_t, data unsafe.Pointer, width, height, stride C.int) {
	// A panic on the capture thread would take the whole process down, so a bad frame is dropped instead.
	defer func() { _ = recover() }()

	cam, ok := cgo.Handle(handle).Value().(*Camera)
	if !ok {
		return
	}

	size := int(stride) * int(height)
	pix := cam.buffer(size)
	copy(pix, unsafe.Slice((*byte)(data), size))

	cam.push(frame.BGRA{Width: int(width), Height: int(height), Stride: int(stride), Pix: pix})
}
