package detect

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	ort "github.com/yalue/onnxruntime_go"

	"coincheur/internal/frame"
	"coincheur/internal/scoring"
)

const (
	ProviderCoreML = "coreml"
	ProviderCPU    = "cpu"
)

type Config struct {
	LibraryPath  string
	ModelPath    string
	Provider     string
	ComputeUnits string
	MinScore     float32
}

func DefaultProvider() string {
	if runtime.GOOS == "darwin" {
		return ProviderCoreML
	}

	return ProviderCPU
}

type Detector struct {
	mutex    sync.Mutex
	options  *ort.SessionOptions
	session  *ort.DynamicAdvancedSession
	input    *ort.Tensor[float32]
	size     int
	deck     []scoring.Card
	minScore float32
}

func New(cfg Config) (*Detector, error) {
	ort.SetSharedLibraryPath(cfg.LibraryPath)

	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("initializing onnxruntime: %w", err)
	}

	detector, err := open(cfg)
	if err != nil {
		return nil, errors.Join(err, ort.DestroyEnvironment())
	}

	return detector, nil
}

func open(cfg Config) (*Detector, error) {
	deck := scoring.Deck()

	inputs, outputs, err := ort.GetInputOutputInfo(cfg.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("reading model signature: %w", err)
	}

	if len(inputs) != 1 || len(outputs) != 1 {
		return nil, fmt.Errorf("unexpected model signature: %d inputs and %d outputs", len(inputs), len(outputs))
	}

	if len(inputs[0].Dimensions) != 4 || len(outputs[0].Dimensions) != 3 {
		return nil, fmt.Errorf("unexpected tensor ranks %v and %v", inputs[0].Dimensions, outputs[0].Dimensions)
	}

	if classes := outputs[0].Dimensions[1] - boxValues; int(classes) != len(deck) {
		return nil, fmt.Errorf("model has %d classes, expected %d", classes, len(deck))
	}

	options, err := newOptions(cfg)
	if err != nil {
		return nil, err
	}

	session, err := ort.NewDynamicAdvancedSession(cfg.ModelPath, []string{inputs[0].Name}, []string{outputs[0].Name}, options)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("creating session: %w", err), options.Destroy())
	}

	size := int(inputs[0].Dimensions[2])

	input, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 3, int64(size), int64(size)))
	if err != nil {
		return nil, errors.Join(fmt.Errorf("creating input tensor: %w", err), session.Destroy(), options.Destroy())
	}

	return &Detector{
		options:  options,
		session:  session,
		input:    input,
		size:     size,
		deck:     deck,
		minScore: cfg.MinScore,
	}, nil
}

func newOptions(cfg Config) (*ort.SessionOptions, error) {
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("creating session options: %w", err)
	}

	switch cfg.Provider {
	case ProviderCPU:
	case ProviderCoreML:
		err = options.AppendExecutionProviderCoreMLV2(map[string]string{
			"ModelFormat":    "MLProgram",
			"MLComputeUnits": cfg.ComputeUnits,
		})
		if err != nil {
			return nil, errors.Join(fmt.Errorf("enabling coreml: %w", err), options.Destroy())
		}
	default:
		return nil, errors.Join(fmt.Errorf("unknown provider %q", cfg.Provider), options.Destroy())
	}

	return options, nil
}

func (d *Detector) Detect(img frame.BGRA) ([]Detection, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	fit := newLetterbox(img.Width, img.Height, d.size)
	fit.fill(img, d.input.GetData())

	outputs := []ort.Value{nil}
	if err := d.session.Run([]ort.Value{d.input}, outputs); err != nil {
		return nil, fmt.Errorf("running model: %w", err)
	}

	output, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output type %T", outputs[0])
	}

	defer func() { _ = output.Destroy() }()

	shape := output.GetShape()
	candidates := suppress(decode(output.GetData(), int(shape[2]), len(d.deck), d.minScore))

	return fit.detections(candidates, d.deck, img.Width, img.Height), nil
}

func (d *Detector) Close() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return errors.Join(d.input.Destroy(), d.session.Destroy(), d.options.Destroy(), ort.DestroyEnvironment())
}
