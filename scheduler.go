package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	
)

// StartScheduler запускает планировщик перезагрузок
func StartScheduler() {
	cfg := GetServersConfig()
	
	if !cfg.SchedulerEnabled {
		fmt.Println("⏰ Планировщик перезагрузок отключен")
		return
	}
	
	if len(cfg.RestartTimes) == 0 {
		fmt.Println("⏰ Планировщик: нет времени для перезагрузок")
		return
	}
	
	fmt.Printf("⏰ Планировщик перезагрузок запущен\n")
	fmt.Printf("   Перезагрузки в: %v\n", cfg.RestartTimes)
	fmt.Printf("   Условие: перезагружается только сервер, где есть точки с задержкой\n")
	
	go func() {
		for {
			now := time.Now()
			
			var nextRun time.Time
			for _, timeStr := range cfg.RestartTimes {
				runTime := getNextRunTime(now, timeStr)
				if nextRun.IsZero() || runTime.Before(nextRun) {
					nextRun = runTime
				}
			}
			
			waitDuration := nextRun.Sub(now)
			log.Printf("⏰ Следующая проверка условий через %v (в %s)", waitDuration, nextRun.Format("15:04"))
			
			select {
			case <-time.After(waitDuration):
				checkAndExecuteRestart(nextRun)
			case <-shutdown:
				fmt.Println("⏰ Планировщик остановлен")
				return
			}
		}
	}()
}

// checkAndExecuteRestart проверяет условия и перезагружает только проблемные серверы
func checkAndExecuteRestart(runTime time.Time) {
	log.Printf("🔍 ПРОВЕРКА УСЛОВИЙ для перезагрузки в %s", runTime.Format("15:04:05"))
	
	cfg := GetServersConfig()
	restartedCount := 0
	
	for _, server := range cfg.Servers {
		delaysCount, err := getDelayedPointsCount(server.Address)
		if err != nil {
			log.Printf("   ⚠️ Сервер %s: ошибка проверки - %v", server.Name, err)
			continue
		}
		
		log.Printf("   📊 Сервер %s: точек с задержкой = %d", server.Name, delaysCount)
		
		if delaysCount > 0 {
			log.Printf("   🔄 На сервере %s обнаружены задержки (%d точек), выполняю перезагрузку...", server.Name, delaysCount)
			success := restartServer(server.Address)
			if success {
				log.Printf("   ✅ Сервер %s перезагружен успешно", server.Name)
				restartedCount++
			} else {
				log.Printf("   ❌ Ошибка перезагрузки сервера %s", server.Name)
			}
			time.Sleep(5 * time.Second)
		} else {
			log.Printf("   ⏭️ Сервер %s: задержек нет, перезагрузка не требуется", server.Name)
		}
	}
	
	if restartedCount == 0 {
		log.Printf("⏭️ ПЕРЕЗАГРУЗКИ НЕ ВЫПОЛНЕНЫ: нет серверов с задержками")
	} else {
		log.Printf("✅ Плановая перезагрузка завершена. Перезагружено серверов: %d", restartedCount)
	}
}

// getDelayedPointsCount получает количество точек с задержкой
func getDelayedPointsCount(serverURL string) (int, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(serverURL + "/IServer/data-delays")
	if err != nil {
		return 0, fmt.Errorf("ошибка подключения: %v", err)
	}
	defer resp.Body.Close()
	
	var response DataDelaysResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, fmt.Errorf("ошибка парсинга: %v", err)
	}
	
	if !response.Success {
		return 0, fmt.Errorf("сервер вернул ошибку")
	}
	
	return response.DelayedPointsCount, nil
}

func getNextRunTime(now time.Time, timeStr string) time.Time {
	t, _ := time.Parse("15:04", timeStr)
	next := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	if next.Before(now) || next.Equal(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func restartServer(serverURL string) bool {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(serverURL+"/restart", "application/json", nil)
	if err != nil {
		log.Printf("   Ошибка подключения к %s: %v", serverURL, err)
		return false
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == 200
}