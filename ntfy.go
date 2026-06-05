package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type NtfyMessage struct {
	Topic    string `json:"topic"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

// StartNtfyNotifier запускает планировщик ntfy уведомлений
func StartNtfyNotifier() {
	cfg := GetServersConfig()

	if !cfg.Ntfy.Enabled {
		fmt.Println("🔔 Ntfy уведомления отключены")
		return
	}

	fmt.Printf("🔔 Ntfy уведомления запущены (раз в 2 часа, %02d:00-%02d:00)\n",
		cfg.Ntfy.StartHour, cfg.Ntfy.EndHour)

	// Запускаем ежесуточную проверку в 12:00
	go startDailyHealthCheck()

	go func() {
		ticker := time.NewTicker(120 * time.Minute) // раз в 2 часа
		defer ticker.Stop()

		time.Sleep(30 * time.Second)
		checkAndSendNtfy()

		for {
			select {
			case <-ticker.C:
				checkAndSendNtfy()
			case <-shutdown:
				fmt.Println("🔔 Ntfy уведомления остановлены")
				return
			}
		}
	}()
}

// startDailyHealthCheck ежесуточная проверка в 12:00
func startDailyHealthCheck() {
	for {
		now := time.Now()
		// Вычисляем время до следующего 12:00
		next := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		waitDuration := next.Sub(now)
		time.Sleep(waitDuration)

		// Отправляем тестовое уведомление
		sendHealthCheckNotification()
	}
}

// sendHealthCheckNotification отправляет ежесуточное уведомление о здоровье системы
func sendHealthCheckNotification() {
	cfg := GetServersConfig()

	// Проверяем рабочее время (чтобы не беспокоить ночью)
	now := time.Now()
	currentHour := now.Hour()
	if currentHour < cfg.Ntfy.StartHour || currentHour >= cfg.Ntfy.EndHour {
		return
	}

	status := GetFullStatus()

	// Собираем общую статистику
	var problemsCount int
	var serversStatus []string

	for _, server := range status.Servers {
		serverOk := true
		problems := []string{}

		if server.TimeStatus == "critical" || server.TimeStatus == "error" {
			serverOk = false
			problems = append(problems, "время")
		}
		if server.XmlStatus != "" && strings.Contains(server.XmlStatus, "❌") && !strings.Contains(server.XmlStatus, "Не удалось распознать дату") {
			serverOk = false
			problems = append(problems, "XML")
		}
		if server.HasDataDelays && server.DelayedPoints > 0 {
			serverOk = false
			problems = append(problems, fmt.Sprintf("задержки(%d)", server.DelayedPoints))
		}

		if !serverOk {
			problemsCount++
			serversStatus = append(serversStatus, fmt.Sprintf("• %s: %s", server.Name, strings.Join(problems, ", ")))
		}
	}

	var title, message string
	if problemsCount == 0 {
		title = "✅ Система в норме"
		message = fmt.Sprintf("Все серверы работают корректно\nВремя: %s", now.Format("15:04:05"))
	} else {
		title = "⚠️ Ежесуточный отчет"
		message = fmt.Sprintf("Обнаружены проблемы на %d сервере(ах):\n%s\n\nВремя: %s",
			problemsCount, strings.Join(serversStatus, "\n"), now.Format("15:04:05"))
	}

	SendNtfyNotification(title, message)
}

// checkAndSendNtfy проверяет статусы и отправляет уведомления
func checkAndSendNtfy() {
	cfg := GetServersConfig()

	// Проверяем рабочее время
	now := time.Now()
	currentHour := now.Hour()
	if currentHour < cfg.Ntfy.StartHour || currentHour >= cfg.Ntfy.EndHour {
		return
	}

	status := GetFullStatus()

	for _, server := range status.Servers {
		// Исключаем сервер Техучет (10.96.30.61)
		if strings.Contains(server.Address, "10.96.30.61") {
			continue
		}
		// Проверка времени (critical или error)
		if server.TimeStatus == "critical" || server.TimeStatus == "error" {
			var diffMsg string
			if server.TimeStatus == "error" {
				diffMsg = "ошибка получения времени"
			} else {
				// Конвертируем наносекунды в миллисекунды
				ms := float64(server.TimeDiff) / 1000000
				diffMsg = fmt.Sprintf("%.1f сек", ms/1000)
			}
			SendNtfyNotification("⚠️ Проблема времени",
				fmt.Sprintf("Сервер: %s\nРазница: %s", server.Name, diffMsg))
		}

		// Проверка XML (есть ошибка)
		if server.XmlStatus != "" && strings.Contains(server.XmlStatus, "❌") {
			title := "❌ Проблема XML"
			message := fmt.Sprintf("Сервер: %s\nСтатус: %s", server.Name, server.XmlStatus)
			SendNtfyNotification(title, message)
		}

		// Проверка задержек данных
		if server.HasDataDelays && server.DelayedPoints > 0 {
			var pointsInfo string
			if len(server.DelayedPointsList) > 0 {
				// Показываем первые 3 точки
				pointsInfo = "\n\nТочки с задержкой:"
				for i, point := range server.DelayedPointsList {
					if i >= 3 {
						pointsInfo += fmt.Sprintf("\n... и еще %d", server.DelayedPoints-3)
						break
					}
					pointsInfo += fmt.Sprintf("\n• %s (задержка: %.1f ч)", point.Name, point.DelayHours)
				}
			}

			SendNtfyNotification("⚠️ Задержки данных",
				fmt.Sprintf("Сервер: %s\nКоличество точек с задержкой: %d%s",
					server.Name, server.DelayedPoints, pointsInfo))
		}
	}
}

// SendNtfyNotification отправляет уведомление на ntfy сервер
func SendNtfyNotification(title, message string) {
	cfg := GetServersConfig()

	url := cfg.Ntfy.Server + "/" + cfg.Ntfy.Topic

	msg := NtfyMessage{
		Topic:    cfg.Ntfy.Topic,
		Title:    title,
		Message:  message,
		Priority: cfg.Ntfy.Priority,
	}

	jsonData, _ := json.Marshal(msg)

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("[ERR] Создание запроса: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[ERR] Отправка ntfy: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("[OK] Ntfy уведомление отправлено: %s\n", title)
	} else {
		fmt.Printf("[WARN] Ntfy HTTP %d\n", resp.StatusCode)
	}
}
