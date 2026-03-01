package profile

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFromFile reads a YAML profile from disk and returns it.
func LoadFromFile(path string) (p *Profile, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is user-provided profile file location
	if err != nil {
		return nil, fmt.Errorf("read profile file: %w", err)
	}

	p = &Profile{}
	if err = yaml.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("unmarshal YAML profile: %w", err)
	}
	return p, nil
}
