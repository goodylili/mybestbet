package logos

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadSourceLogoURLs(configPath string) (map[string]string, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", configPath, err)
	}
	var file struct {
		Sources map[string]struct {
			LogoURL string `yaml:"logoUrl"`
		} `yaml:"sources"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse sources block of %s: %w", configPath, err)
	}
	urls := map[string]string{}
	for name, source := range file.Sources {
		if source.LogoURL != "" {
			urls[name] = source.LogoURL
		}
	}
	return urls, nil
}
