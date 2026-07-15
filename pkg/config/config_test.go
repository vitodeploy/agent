package config

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDecodeConfigWithoutServices(t *testing.T) {
	config := &Config{}
	input := `{"url":"https://vito.example/api/servers/1/agent/12","secret":"a-uuid"}`

	if err := json.Unmarshal([]byte(input), config); err != nil {
		t.Fatalf("decoding failed: %v", err)
	}

	if config.Url != "https://vito.example/api/servers/1/agent/12" {
		t.Errorf("got url %q", config.Url)
	}
	if config.Secret != "a-uuid" {
		t.Errorf("got secret %q", config.Secret)
	}
	if config.Services != nil {
		t.Errorf("expected nil services, got %#v", config.Services)
	}
}

func TestDecodeConfigWithServices(t *testing.T) {
	config := &Config{}
	input := `{"url":"https://vito.example","secret":"a-uuid","services":[{"id":3,"unit":"nginx"},{"id":7,"unit":"php8.4-fpm"}]}`

	if err := json.Unmarshal([]byte(input), config); err != nil {
		t.Fatalf("decoding failed: %v", err)
	}

	expected := []ServiceConfig{{Id: 3, Unit: "nginx"}, {Id: 7, Unit: "php8.4-fpm"}}
	if len(config.Services) != len(expected) {
		t.Fatalf("expected %d services, got %d", len(expected), len(config.Services))
	}
	for i, service := range expected {
		if config.Services[i] != service {
			t.Errorf("service %d: expected %#v, got %#v", i, service, config.Services[i])
		}
	}
}

func TestDecodeConfigWithNullServices(t *testing.T) {
	config := &Config{}
	input := `{"url":"https://vito.example","secret":"a-uuid","services":null}`

	if err := json.Unmarshal([]byte(input), config); err != nil {
		t.Fatalf("decoding failed: %v", err)
	}

	if config.Services != nil {
		t.Errorf("expected nil services, got %#v", config.Services)
	}
}

// Configs written by newer Vito versions may contain keys this agent does not
// know about.
func TestDecodeConfigIgnoresUnknownFields(t *testing.T) {
	config := &Config{}
	input := `{"url":"https://vito.example","secret":"a-uuid","something_new":{"a":1}}`

	if err := json.Unmarshal([]byte(input), config); err != nil {
		t.Fatalf("decoding failed: %v", err)
	}

	if config.Url != "https://vito.example" {
		t.Errorf("got url %q", config.Url)
	}
}

// The self-created default config file must stay identical to the one older
// agent versions wrote.
func TestEncodeDefaultConfig(t *testing.T) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent("", "    ")
	if err := encoder.Encode(&Config{Url: "", Secret: ""}); err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	expected := "{\n    \"url\": \"\",\n    \"secret\": \"\"\n}\n"
	if buffer.String() != expected {
		t.Errorf("expected %q, got %q", expected, buffer.String())
	}
}
