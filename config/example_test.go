package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestExampleFileMatchesGenerator pins the checked-in config.example.json
// to the generator output: editing either side without the other must
// fail, and the published example must stay a valid configuration.
func TestExampleFileMatchesGenerator(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "config.example.json"))
	if err != nil {
		t.Fatalf("read config.example.json: %v", err)
	}

	generated, err := Example()
	if err != nil {
		t.Fatalf("Example() error = %v", err)
	}

	var fromFile, fromGen Config
	if err := json.Unmarshal(data, &fromFile); err != nil {
		t.Fatalf("config.example.json is not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(generated), &fromGen); err != nil {
		t.Fatalf("Example() output is not valid JSON: %v", err)
	}

	if !reflect.DeepEqual(fromFile, fromGen) {
		t.Errorf("config.example.json drifted from Example() output:\nfile: %+v\ngen:  %+v", fromFile, fromGen)
	}

	if err := fromFile.Validate(); err != nil {
		t.Errorf("config.example.json does not pass Validate(): %v", err)
	}
}
