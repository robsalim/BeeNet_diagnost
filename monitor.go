package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ServerStatus - статус отдельного сервера
type ServerStatus struct {
	Name              string         `json:"name"`
	Address           string         `json:"address"`
	ServerAlive       bool           `json:"server_alive"`
	ServerError       string         `json:"server_error,omitempty"`
	ApiAlive          bool           `json:"api_alive"`
	ApiPid            int            `json:"api_pid"`
	ApiUser           string         `json:"api_user"`
	ApiError          string         `json:"api_error,omitempty"`
	TimeDiff          time.Duration  `json:"time_diff_ns"`
	TimeStatus        string         `json:"time_status"`
	LastCheck         time.Time      `json:"last_check"`
	DbConnected       bool           `json:"db_connected"`
	DbError           string         `json:"db_error,omitempty"`
	XmlStatus         string         `json:"xml_status"`
	HasDataDelays     bool           `json:"has_data_delays"`
	DelayedPoints     int            `json:"delayed_points_count"`
	DelayedPointsList []DelayedPoint `json:"delayed_points_list,omitempty"`
}

// DelayedPoint - точка с задержкой
type DelayedPoint struct {
	Name       string  `json:"name"`
	LastDate   string  `json:"last_date"`
	DelayHours float64 `json:"delay_hours"`
}

// FullStatus - полный статус всех серверов
type FullStatus struct {
	Servers       []ServerStatus `json:"servers"`
	LastCheck     time.Time      `json:"last_check"`
	MonitorUptime string         `json:"monitor_uptime"`
	SystemTime    time.Time      `json:"system_time"`
}

var (
	status      FullStatus
	statusMutex sync.RWMutex
	startTime   = time.Now()
)

// StartMonitoring запускает мониторинг
func StartMonitoring() {
	cfg := GetServersConfig()

	fmt.Println("🔍 Запуск мониторинга...")

	// Инициализируем статусы серверов
	statusMutex.Lock()
	status.Servers = make([]ServerStatus, len(cfg.Servers))
	for i, s := range cfg.Servers {
		status.Servers[i] = ServerStatus{
			Name:    s.Name,
			Address: s.Address,
		}
	}
	statusMutex.Unlock()

	// Первая проверка сразу
	checkAllServers()

	// Запускаем периодические проверки
	ticker := time.NewTicker(time.Duration(cfg.CheckIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-shutdown:
			fmt.Println("🛑 Мониторинг остановлен")
			return
		case <-ticker.C:
			checkAllServers()
		}
	}
}

// checkAllServers - проверка всех серверов
func checkAllServers() {
	cfg := GetServersConfig()

	statusMutex.Lock()
	defer statusMutex.Unlock()

	// Обновляем время
	status.MonitorUptime = time.Since(startTime).Truncate(time.Second).String()
	status.SystemTime = time.Now()

	// Проверяем каждый сервер
	for i := range status.Servers {
		checkServer(&status.Servers[i], cfg)
	}

	status.LastCheck = time.Now()
}

// checkServer - проверка одного сервера
func checkServer(server *ServerStatus, cfg ServersConfig) {
	server.LastCheck = time.Now()

	// 1. Проверяем статус IServer
	serverAlive, err := checkIServerStatus(server.Address)
	server.ServerAlive = serverAlive
	if err != nil {
		server.ServerError = err.Error()
	} else {
		server.ServerError = ""
	}

	// 2. Проверяем Web_Bee.Api (права доступа)
	apiInfo, err := getPrivilegesForServer(server.Address)
	if err != nil {
		server.ApiAlive = false
		server.ApiError = err.Error()
	} else {
		server.ApiAlive = true
		server.ApiPid = apiInfo.ProcessId
		server.ApiUser = apiInfo.User
		server.ApiError = ""
	}

	// 3. Проверяем разницу времени
	timeDiff, err := getTimeDifferenceForServer(server.Address)
	if err == nil {
		server.TimeDiff = timeDiff
		
		absDiff := timeDiff
		if absDiff < 0 {
			absDiff = -absDiff
		}
		threshold := time.Duration(cfg.TimeDiffThreshold) * time.Second

		if absDiff < 1*time.Second {
			server.TimeStatus = "good"
		} else if absDiff < threshold {
			server.TimeStatus = "warning"
		} else {
			server.TimeStatus = "critical"
		}
	} else {
		server.TimeStatus = "error"
	}

	// 4. Проверяем БД (если сервер доступен)
	if server.ServerAlive {
		checkDatabase(server)
	}
}

// checkIServerStatus проверяет статус IServer
func checkIServerStatus(serverURL string) (bool, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serverURL + "/health")
	if err != nil {
		return false, fmt.Errorf("ошибка подключения: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, fmt.Errorf("HTTP статус: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("ошибка чтения: %v", err)
	}

	return string(body) == "🟢 IServer.exe запущен", nil
}

// PrivilegesInfo - информация о правах доступа
type PrivilegesInfo struct {
	User             string `json:"user"`
	IsAdministrator  bool   `json:"isAdministrator"`
	ProcessId        int    `json:"processId"`
	CurrentDirectory string `json:"currentDirectory"`
	Error            string `json:"error,omitempty"`
}

// getPrivilegesForServer - получение информации о Web API
func getPrivilegesForServer(serverURL string) (PrivilegesInfo, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(serverURL + "/debug/privileges")
	if err != nil {
		return PrivilegesInfo{}, fmt.Errorf("ошибка подключения: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return PrivilegesInfo{}, fmt.Errorf("ошибка чтения: %v", err)
	}

	var info PrivilegesInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return PrivilegesInfo{}, fmt.Errorf("ошибка парсинга: %v", err)
	}

	return info, nil
}

// getTimeDifferenceForServer - разница времени
func getTimeDifferenceForServer(serverURL string) (time.Duration, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serverURL + "/time-diff")
	if err != nil {
		return 0, fmt.Errorf("ошибка связи: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("ошибка чтения: %v", err)
	}

	var response struct {
		Success  bool          `json:"success"`
		TimeDiff time.Duration `json:"time_diff_ns"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("ошибка парсинга: %v", err)
	}

	if !response.Success {
		return 0, fmt.Errorf("сервер вернул ошибку")
	}

	return response.TimeDiff, nil
}

// checkDatabase - проверка БД сервера
func checkDatabase(server *ServerStatus) {
	// Получаем health БД
	dbHealth, err := getDatabaseHealthForServer(server.Address)
	if err != nil {
		server.DbConnected = false
		server.DbError = err.Error()
		return
	}

	server.DbConnected = dbHealth.Success
	server.XmlStatus = dbHealth.XmlStatus

	// Получаем задержки данных
	dataDelays, err := getDataDelaysForServer(server.Address)
	if err == nil && dataDelays.Success {
		server.HasDataDelays = dataDelays.HasDelays
		server.DelayedPoints = dataDelays.DelayedPointsCount
		if len(dataDelays.DelayedPoints) > 0 {
			server.DelayedPointsList = dataDelays.DelayedPoints
		}
	}
}

// DatabaseHealthResponse - ответ health БД
type DatabaseHealthResponse struct {
	Success       bool   `json:"success"`
	SqlConnection string `json:"sql_connection"`
	XmlStatus     string `json:"xml_status"`
	LastCheck     string `json:"last_check"`
}

// DataDelaysResponse - ответ о задержках
type DataDelaysResponse struct {
	Success            bool           `json:"success"`
	HasDelays          bool           `json:"has_delays"`
	DelayedPointsCount int            `json:"delayed_points_count"`
	DelayedPoints      []DelayedPoint `json:"delayed_points"`
	Error              string         `json:"error,omitempty"`
	LastCheck          string         `json:"last_check"`
}

// getDatabaseHealthForServer - здоровье БД для сервера
func getDatabaseHealthForServer(serverURL string) (DatabaseHealthResponse, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serverURL + "/db-health")
	if err != nil {
		return DatabaseHealthResponse{Success: false}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DatabaseHealthResponse{Success: false}, err
	}

	var response DatabaseHealthResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return DatabaseHealthResponse{Success: false}, err
	}

	return response, nil
}

// getDataDelaysForServer - задержки данных для сервера
func getDataDelaysForServer(serverURL string) (DataDelaysResponse, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serverURL + "/data-delays")
	if err != nil {
		return DataDelaysResponse{Success: false}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DataDelaysResponse{Success: false}, err
	}

	var response DataDelaysResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return DataDelaysResponse{Success: false}, err
	}

	return response, nil
}

// GetFullStatus возвращает полный статус
func GetFullStatus() FullStatus {
	statusMutex.RLock()
	defer statusMutex.RUnlock()
	return status
}