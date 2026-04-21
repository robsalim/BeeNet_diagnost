package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
)

// ServerConfig - конфигурация отдельного сервера
type ServerConfig struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// ServersConfig - общая конфигурация
type ServersConfig struct {
	Servers              []ServerConfig `json:"servers"`
	WebPort              string         `json:"web_port"`
	CheckIntervalSeconds int            `json:"check_interval_seconds"`
	TimeDiffThreshold    int            `json:"time_diff_threshold_seconds"`
	SchedulerEnabled     bool           `json:"scheduler_enabled"`
	RestartTimes         []string       `json:"restart_times"`
}

var (
	serversConfig ServersConfig
	configMutex   sync.RWMutex
	configPath    = "config/servers.json"
	shutdown      = make(chan bool, 1)
)


// MeterConfig - конфигурация прибора учета
type MeterConfig struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	ModemLocation   string `json:"modem_location"`
	ModemIP         string `json:"modem_ip"`
	ConnectionPort  string `json:"connection_port"`
	MeterType       string `json:"meter_type"`
	MeterNumber     string `json:"meter_number"`
	AccountingType  string `json:"accounting_type"`
}

// MetersConfig - конфигурация приборов учета
type MetersConfig struct {
	Meters []MeterConfig `json:"meters"`
}

var (
	metersConfig MetersConfig
	metersPath   = "config/meters.json"
)



type ModemConfig struct {
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	Name    string `json:"name"`
	Group   string `json:"group"` // группа (5155, 5143, 5167)
}

// ModemsConfig - конфигурация модемов
type ModemsConfig struct {
	Modems  []ModemConfig `json:"modems"`
	Timeout int           `json:"timeout_seconds"` // таймаут проверки в секундах
}

var (
	modemsConfig ModemsConfig
	modemsPath   = "config/modems.json"
)


// LoadMetersConfig загружает конфигурацию приборов учета
func LoadMetersConfig() error {
	if err := os.MkdirAll("config", 0755); err != nil {
		return fmt.Errorf("ошибка создания директории config: %v", err)
	}

	data, err := ioutil.ReadFile(metersPath)
	if err != nil {
		// Создаем пустой файл если нет
		metersConfig = MetersConfig{Meters: []MeterConfig{}}
		return SaveMetersConfig()
	}

	return json.Unmarshal(data, &metersConfig)
}

// SaveMetersConfig сохраняет конфигурацию
func SaveMetersConfig() error {
	data, err := json.MarshalIndent(metersConfig, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(metersPath, data, 0644)
}

// GetMetersConfig возвращает конфигурацию
func GetMetersConfig() MetersConfig {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return metersConfig
}


// LoadModemsConfig загружает конфигурацию модемов
func LoadModemsConfig() error {
	if err := os.MkdirAll("config", 0755); err != nil {
		return fmt.Errorf("ошибка создания директории config: %v", err)
	}

	data, err := ioutil.ReadFile(modemsPath)
	if err != nil {
		// Создаем дефолтную конфигурацию
		modemsConfig = ModemsConfig{
			Modems: []ModemConfig{
				{IP: "172.16.56.154", Port: 5155, Name: "Модем 1", Group: "5155"},
				{IP: "172.16.56.153", Port: 5155, Name: "Модем 2", Group: "5155"},

			},
			Timeout: 3,
		}
		return SaveModemsConfig()
	}
	return json.Unmarshal(data, &modemsConfig)
}

// SaveModemsConfig сохраняет конфигурацию модемов
func SaveModemsConfig() error {
	data, err := json.MarshalIndent(modemsConfig, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(modemsPath, data, 0644)
}

// GetModemsConfig возвращает конфигурацию модемов
func GetModemsConfig() ModemsConfig {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return modemsConfig
}


// LoadServersConfig загружает конфигурацию серверов
func LoadServersConfig() error {
	// Создаем директорию если нет
	if err := os.MkdirAll("config", 0755); err != nil {
		return fmt.Errorf("ошибка создания директории config: %v", err)
	}

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		// Создаем дефолтную конфигурацию
		serversConfig = ServersConfig{
			Servers: []ServerConfig{
				{Name: "Сервер 'Береза' (10.96.30.62)", Address: "http://10.96.30.62:9200", Port: 9200},
				{Name: "Сервер 'Малые точки' (10.98.30.36)", Address: "http://10.98.30.36:9200", Port: 9200},
			},
			WebPort:              ":8090",
			CheckIntervalSeconds: 30,
			TimeDiffThreshold:    3,
			SchedulerEnabled:     true,
			RestartTimes:         []string{"02:50"},   //, "06:50" если несколько раз
		}

		fmt.Println("Создан файл конфигурации config/servers.json")
		fmt.Println("Отредактируйте его при необходимости и перезапустите монитор")

		if err := SaveServersConfig(); err != nil {
			return fmt.Errorf("ошибка создания config/servers.json: %v", err)
		}
	} else {
		if err := json.Unmarshal(data, &serversConfig); err != nil {
			return fmt.Errorf("ошибка парсинга config/servers.json: %v", err)
		}
	}

	fmt.Println("=== ЗАГРУЖЕННАЯ КОНФИГУРАЦИЯ ===")
	fmt.Printf("Веб-порт: %s\n", serversConfig.WebPort)
	fmt.Printf("Интервал проверки: %d сек\n", serversConfig.CheckIntervalSeconds)
	fmt.Printf("Порог разницы времени: %d сек\n", serversConfig.TimeDiffThreshold)
	fmt.Println("Серверы:")
	for i, s := range serversConfig.Servers {
		fmt.Printf("  %d. %s - %s\n", i+1, s.Name, s.Address)
	}
	fmt.Println("================================")

	return nil
}

// SaveServersConfig сохраняет конфигурацию
func SaveServersConfig() error {
	data, err := json.MarshalIndent(serversConfig, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(configPath, data, 0644)
}

// GetServersConfig возвращает конфигурацию
func GetServersConfig() ServersConfig {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return serversConfig
}