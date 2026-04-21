package main

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sync"
	"time"
)


// PingResult - результат пинга
type PingResult struct {
	IP      string `json:"ip"`
	Alive   bool   `json:"alive"`
	Message string `json:"message"`
	Latency string `json:"latency"`
	PingMs  int64  `json:"ping_ms"`
}

// ModemTestResult - результат проверки модема
type ModemTestResult struct {
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	Name    string `json:"name"`
	Group   string `json:"group"`
	Alive   bool   `json:"alive"`
	Message string `json:"message"`
	Latency string `json:"latency"`
	PingMs  int64  `json:"ping_ms"`
}

// TestModemsTCP - проверка модемов по TCP
func TestModemsTCP(timeout int) []ModemTestResult {
	modems := GetModemsConfig()
	results := make([]ModemTestResult, len(modems.Modems))
	var wg sync.WaitGroup
	
	for i, modem := range modems.Modems {
		wg.Add(1)
		go func(idx int, m ModemConfig) {
			defer wg.Done()
			results[idx] = testSingleModemTCP(m, timeout)
		}(i, modem)
	}
	
	wg.Wait()
	return results
}

// TestModemsPing - проверка модемов по PING
func TestModemsPing(timeout int) []ModemTestResult {
	modems := GetModemsConfig()
	results := make([]ModemTestResult, len(modems.Modems))
	var wg sync.WaitGroup
	
	for i, modem := range modems.Modems {
		wg.Add(1)
		go func(idx int, m ModemConfig) {
			defer wg.Done()
			results[idx] = testSingleModemPing(m, timeout)
		}(i, modem)
	}
	
	wg.Wait()
	return results
}

// testSingleModemTCP - проверка одного модема по TCP
func testSingleModemTCP(modem ModemConfig, timeout int) ModemTestResult {
	addr := fmt.Sprintf("%s:%d", modem.IP, modem.Port)
	start := time.Now()
	
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeout)*time.Second)
	latency := time.Since(start)
	
	result := ModemTestResult{
		IP:      modem.IP,
		Port:    modem.Port,
		Name:    modem.Name,
		Group:   modem.Group,
		Latency: latency.String(),
	}
	
	if err != nil {
		result.Alive = false
		result.Message = err.Error()
	} else {
		conn.Close()
		result.Alive = true
		result.Message = "Доступен (TCP)"
	}
	
	return result
}

// testSingleModemPing - проверка одного модема по PING (3 попытки, таймаут 4 сек)
func testSingleModemPing(modem ModemConfig, timeout int) ModemTestResult {
	start := time.Now()
	
	result := ModemTestResult{
		IP:    modem.IP,
		Port:  modem.Port,
		Name:  modem.Name,
		Group: modem.Group,
	}
	
	successCount := 0
	var totalLatency int64
	
	for attempt := 1; attempt <= 5; attempt++ {
		attemptStart := time.Now()
		var err error
		
		if runtime.GOOS == "windows" {
			// Windows: 1 пакет, таймаут 5000 мс
			cmd := exec.Command("ping", "-n", "1", "-w", "5000", modem.IP)
			err = cmd.Run()
		} else {
			// Linux: 1 пакет, таймаут 5 секунды
			cmd := exec.Command("ping", "-c", "1", "-W", "5", modem.IP)
			err = cmd.Run()
		}
		
		attemptLatency := time.Since(attemptStart).Milliseconds()
		
		if err == nil {
			successCount++
			totalLatency += attemptLatency
		}
		
		// Небольшая пауза между попытками
		if attempt < 5 {
			time.Sleep(1000 * time.Millisecond)
		}
	}
	
	latency := time.Since(start)
	result.Latency = latency.String()
	
	if successCount > 0 {
		result.Alive = true
		avgLatency := totalLatency / int64(successCount)
		result.PingMs = avgLatency
		result.Message = fmt.Sprintf("✅ Доступен (PING: %d мс, успешно: %d/5)", avgLatency, successCount)
	} else {
		result.Alive = false
		result.PingMs = 0
		result.Message = "❌ PING: не отвечает (5 попыток)"
	}
	
	return result
}

// PingSingleIP - пинг одного IP адреса (таймаут 3 сек)
func PingSingleIP(ip string) PingResult {
	start := time.Now()
	
	result := PingResult{
		IP: ip,
	}
	
	var err error
	
	if runtime.GOOS == "windows" {
		// Windows: 1 пакет, таймаут 3000 мс
		cmd := exec.Command("ping", "-n", "1", "-w", "3000", ip)
		err = cmd.Run()
	} else {
		// Linux: 1 пакет, таймаут 3 секунды
		cmd := exec.Command("ping", "-c", "1", "-W", "3", ip)
		err = cmd.Run()
	}
	
	latency := time.Since(start)
	result.Latency = latency.String()
	result.PingMs = latency.Milliseconds()
	
	if err != nil {
		result.Alive = false
		result.Message = "❌ Не отвечает"
	} else {
		result.Alive = true
		result.Message = fmt.Sprintf("✅ Доступен (задержка: %d мс)", result.PingMs)
	}
	
	return result
}