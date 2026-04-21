package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"net"
	"fmt"
)

// CommandResponse - ответ на команду
type CommandResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}


// MeterCheckResult - результат проверки счетчика
type MeterCheckResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	Addr    byte   `json:"addr"`
	RawData string `json:"raw_data,omitempty"`
}


// handleFullStatusAPI - полный статус всех серверов
func handleFullStatusAPI(w http.ResponseWriter, r *http.Request) {
	status := GetFullStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleMetersAPI - получение списка приборов учета
func handleMetersAPI(w http.ResponseWriter, r *http.Request) {
	meters := GetMetersConfig()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"meters":  meters.Meters,
		"count":   len(meters.Meters),
	})
}

// handleAllServersStatusAPI - статус всех серверов (упрощенный)
func handleAllServersStatusAPI(w http.ResponseWriter, r *http.Request) {
	status := GetFullStatus()
	
	servers := make([]map[string]interface{}, len(status.Servers))
	for i, s := range status.Servers {
		servers[i] = map[string]interface{}{
			"name":         s.Name,
			"address":      s.Address,
			"server_alive": s.ServerAlive,
			"server_error": s.ServerError,
			"api_alive":    s.ApiAlive,
			"api_pid":      s.ApiPid,
			"api_user":     s.ApiUser,
			"api_error":    s.ApiError,
			"time_diff_ms": float64(s.TimeDiff) / 1000000,
			"time_status":  s.TimeStatus,
			"last_check":   s.LastCheck,
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"servers": servers,
	})
}

// handleServerStatusAPI - статус конкретного сервера
func handleServerStatusAPI(w http.ResponseWriter, r *http.Request) {
	serverID := r.URL.Query().Get("id")
	status := GetFullStatus()
	
	var server *ServerStatus
	for i := range status.Servers {
		if status.Servers[i].Name == serverID || status.Servers[i].Address == serverID {
			server = &status.Servers[i]
			break
		}
	}
	
	if server == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Сервер не найден",
		})
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"server":  server,
	})
}

// handlePrivilegesAPI - получение информации о Web API для сервера
func handlePrivilegesAPI(w http.ResponseWriter, r *http.Request) {
	serverURL := r.URL.Query().Get("server")
	if serverURL == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Не указан сервер",
		})
		return
	}
	
	info, err := getPrivilegesForServer(serverURL)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleDatabaseHealthAPI - здоровье БД для сервера
func handleDatabaseHealthAPI(w http.ResponseWriter, r *http.Request) {
	serverURL := r.URL.Query().Get("server")
	if serverURL == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Не указан сервер",
		})
		return
	}
	
	dbHealth, err := getDatabaseHealthForServer(serverURL)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dbHealth)
}

// handleDataDelaysAPI - задержки данных для сервера
func handleDataDelaysAPI(w http.ResponseWriter, r *http.Request) {
	serverURL := r.URL.Query().Get("server")
	if serverURL == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Не указан сервер",
		})
		return
	}
	
	dataDelays, err := getDataDelaysForServer(serverURL)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dataDelays)
}

// handleRestartAPI - перезапуск IServer на указанном сервере
func handleRestartAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Только POST метод", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Server string `json:"server"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Ошибка разбора JSON",
		})
		return
	}

	if req.Server == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Не указан сервер",
		})
		return
	}

	var response CommandResponse

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Post(req.Server+"/restart", "application/json", bytes.NewBuffer([]byte("{}")))
	if err != nil {
		response.Success = false
		response.Message = "Ошибка связи: " + err.Error()
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &response)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleStopAPI - остановка IServer на указанном сервере
func handleStopAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Только POST метод", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Server string `json:"server"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Ошибка разбора JSON",
		})
		return
	}

	if req.Server == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Не указан сервер",
		})
		return
	}

	var response CommandResponse

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(req.Server+"/restart?stopOnly=true", "application/json", bytes.NewBuffer([]byte("{}")))
	if err != nil {
		response.Success = false
		response.Message = "Ошибка связи: " + err.Error()
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &response)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleModemsTCPTestAPI - тест модемов по TCP
func handleModemsTCPTestAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Только POST метод", http.StatusMethodNotAllowed)
		return
	}
	
	cfg := GetModemsConfig()
	results := TestModemsTCP(cfg.Timeout)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"results": results,
		"type":    "tcp",
		"timeout": cfg.Timeout,
	})
}

// handleModemsPingTestAPI - тест модемов по PING
func handleModemsPingTestAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Только POST метод", http.StatusMethodNotAllowed)
		return
	}
	
	cfg := GetModemsConfig()
	results := TestModemsPing(cfg.Timeout)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"results": results,
		"type":    "ping",
		"timeout": cfg.Timeout,
	})
}

// handlePingAPI - пинг одного IP
func handlePingAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Только POST метод", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IP string `json:"ip"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Ошибка разбора JSON",
		})
		return
	}

	if req.IP == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Не указан IP адрес",
		})
		return
	}

	result := PingSingleIP(req.IP)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"result":  result,
	})
}

// handleCheckMeter - проверка наличия счетчика
func handleCheckMeter(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Только POST метод", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IP   string `json:"ip"`
		Port int    `json:"port"`
		Addr byte   `json:"addr"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Ошибка разбора JSON",
		})
		return
	}

	result := checkMeterPresence(req.IP, req.Port, req.Addr)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// checkMeterPresence - проверка наличия счетчика с диагностикой
func checkMeterPresence(ip string, port int, addr byte) MeterCheckResult {
	result := MeterCheckResult{
		IP:   ip,
		Port: port,
		Addr: addr,
	}
	
	// 1. Проверка подключения к модему
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), 5*time.Second)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("❌ Модем не доступен: %v", err)
		return result
	}
	defer conn.Close()
	result.Message = "✅ Модем доступен"
	
	// 2. Формирование запроса
	request := []byte{addr, 0x00}
	crc := calculateCRC(request)
	request = append(request, byte(crc&0xFF), byte(crc>>8))
	
	// 3. Отправка запроса
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err = conn.Write(request)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("❌ Ошибка отправки: %v", err)
		return result
	}
	
	// 4. Чтение ответа
	buf := make([]byte, 100)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("❌ Счетчик НЕ ОТВЕЧАЕТ (таймаут 3 сек): %v", err)
		result.RawData = ""
		return result
	}
	
	// 5. Анализ ответа
	response := buf[:n]
	result.RawData = fmt.Sprintf("% X", response)
	
	// Проверка длины ответа
	if n < 4 {
		result.Success = false
		result.Message = fmt.Sprintf("❌ Ответ слишком короткий (%d байт, ожидается минимум 4)", n)
		return result
	}
	
	// Проверка адреса
	if response[0] != addr {
		result.Success = false
		result.Message = fmt.Sprintf("❌ Неверный адрес в ответе (ожидался 0x%02X, получен 0x%02X)", addr, response[0])
		return result
	}
	
	// Проверка команды
	if response[1] != 0x00 {
		result.Success = false
		result.Message = fmt.Sprintf("❌ Неверная команда в ответе (ожидалась 0x00, получена 0x%02X)", response[1])
		return result
	}
	
	// Проверка CRC
	if n >= 4 {
		receivedCRC := uint16(response[n-1])<<8 | uint16(response[n-2])
		calculatedCRC := calculateCRC(response[:n-2])
		if receivedCRC != calculatedCRC {
			result.Success = false
			result.Message = fmt.Sprintf("❌ Ошибка CRC (получен 0x%04X, вычислен 0x%04X)", receivedCRC, calculatedCRC)
			return result
		}
	}
	
	// Все проверки пройдены
	result.Success = true
	result.Message = fmt.Sprintf("✅ Счетчик В СЕТИ! (ответ: %d байт)", n)
	
	// Если есть данные - добавим расшифровку
	if n > 4 {
		dataBytes := response[2 : n-2]
		result.Message += fmt.Sprintf(" Данные: % X", dataBytes)
	}
	
	return result
}

// calculateCRC - расчет CRC16
func calculateCRC(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&0x0001 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc = crc >> 1
			}
		}
	}
	return crc
}