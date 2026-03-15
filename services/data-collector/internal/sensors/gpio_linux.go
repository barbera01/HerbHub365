package sensors

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type GPIOPin struct {
	number   int
	basePath string
	value    string
}

func OpenGPIOPin(basePath string, number int, direction string) (*GPIOPin, error) {
	path := filepath.Join(basePath, fmt.Sprintf("gpio%d", number))
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(basePath, "export"), []byte(fmt.Sprintf("%d", number)), 0o644); err != nil {
			return nil, err
		}
		deadline := time.Now().Add(500 * time.Millisecond)
		for {
			if _, statErr := os.Stat(path); statErr == nil {
				break
			}
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("gpio%d export timed out", number)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := os.WriteFile(filepath.Join(path, "direction"), []byte(direction), 0o644); err != nil {
		return nil, err
	}

	return &GPIOPin{
		number:   number,
		basePath: basePath,
		value:    filepath.Join(path, "value"),
	}, nil
}

func (p *GPIOPin) Write(state int) error {
	value := "0"
	if state != 0 {
		value = "1"
	}
	return os.WriteFile(p.value, []byte(value), 0o644)
}

func (p *GPIOPin) Read() (int, error) {
	contents, err := os.ReadFile(p.value)
	if err != nil {
		return 0, err
	}
	trimmed := strings.TrimSpace(string(contents))
	if trimmed == "1" {
		return 1, nil
	}
	return 0, nil
}

func (p *GPIOPin) Close() error {
	unexport := filepath.Join(p.basePath, "unexport")
	if err := os.WriteFile(unexport, []byte(fmt.Sprintf("%d", p.number)), 0o644); err != nil {
		return nil
	}
	return nil
}
