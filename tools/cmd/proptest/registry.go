package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type implSpec struct {
	Dir        string            `json:"dir"`
	Language   string            `json:"language"`
	Build      []string          `json:"build"`
	Run        []string          `json:"run"`
	Env        map[string]string `json:"env"`
	HealthPath string            `json:"health_path"`
	Verifier   string            `json:"verifier"`
	Status     string            `json:"status"`
}

type registry struct {
	Impls map[string]implSpec `json:"impls"`
}

func loadRegistry(path string) (registry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return registry{}, err
	}
	var r registry
	if err := json.Unmarshal(b, &r); err != nil {
		return registry{}, err
	}
	if len(r.Impls) == 0 {
		return registry{}, fmt.Errorf("registry %s has no implementations", path)
	}
	return r, nil
}
