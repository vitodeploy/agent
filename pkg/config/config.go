package config

import (
	"encoding/json"
	"fmt"
	"os"
)

const configDir = "/etc/vito-agent"
const configFile = "config.json"

type ServiceConfig struct {
	Id   int64  `json:"id"`
	Unit string `json:"unit"`
}

type Config struct {
	Url    string `json:"url"`
	Secret string `json:"secret"`
	// Services is optional and is absent in configs written by older Vito versions.
	// omitempty keeps the self-created default config file identical to previous versions.
	Services []ServiceConfig `json:"services,omitempty"`
}

func GetConfig() *Config {
	config := &Config{
		Url:    "",
		Secret: "",
	}

	// Create the config directory if it doesn't exist
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		err := os.Mkdir(configDir, 0755)
		if err != nil {
			fmt.Println("Error creating directory:", err)
			panic(err)
		}
	}

	configPath := fmt.Sprintf("%s/%s", configDir, configFile)

	// Check if the config file exists
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		// Create the config file if it doesn't exist
		createFile, err := os.Create(configPath)
		if err != nil {
			fmt.Println("Error creating file:", err)
			panic(err)
		}
		encoder := json.NewEncoder(createFile)
		encoder.SetIndent("", "    ")
		err = encoder.Encode(config)
		if err != nil {
			fmt.Println("Error encoding JSON:", err)
			panic(err)
		}
		createFile.Close()
		return config
	}
	if err != nil {
		fmt.Println("Error checking if file exists:", err)
		panic(err)
	}

	file, err := os.Open(configPath)
	if err != nil {
		fmt.Println("Error opening or creating file:", err)
		panic(err)
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(config)
	if err != nil {
		fmt.Println("Error decoding config:", err)
		panic(err)
	}

	return config
}
