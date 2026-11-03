package detect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	LibraryEnv = "COINCHEUR_ORT_LIB"
	ModelEnv   = "COINCHEUR_MODEL"

	libraryName = "libonnxruntime.dylib"
	modelName   = "corners.onnx"
)

// LocateLibrary looks for ONNX Runtime in the environment, in the app bundle, then in the dev folder.
func LocateLibrary() (string, error) {
	return locate(LibraryEnv, "../Frameworks", filepath.Join("build", "lib"), libraryName)
}

func LocateModel() (string, error) {
	return locate(ModelEnv, "../Resources", filepath.Join("build", "models"), modelName)
}

func locate(envName, bundleFolder, devFolder, fileName string) (string, error) {
	var candidates []string

	if value := os.Getenv(envName); value != "" {
		candidates = append(candidates, value)
	}

	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), bundleFolder, fileName))
	}

	candidates = append(candidates, filepath.Join(devFolder, fileName))

	return firstExisting(candidates)
}

func firstExisting(candidates []string) (string, error) {
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("file not found, tried %s", strings.Join(candidates, ", "))
}
