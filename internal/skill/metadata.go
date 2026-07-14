package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const MetadataFilename = ".skill-metadata.json"

func WriteMetadata(dir string, meta Metadata) error {
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metaPath := filepath.Join(dir, MetadataFilename)
	err = os.WriteFile(metaPath, b, 0644)
	if err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}
	return nil
}

func ReadMetadata(dir string) (Metadata, error) {
	var meta Metadata
	metaPath := filepath.Join(dir, MetadataFilename)
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return meta, err
	}

	if err := json.Unmarshal(b, &meta); err != nil {
		return meta, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return meta, nil
}
