package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var mqttClient mqtt.Client

// StartMQTTNotifier запускает MQTT уведомления
func StartMQTTNotifier() {
	cfg := GetServersConfig()

	if !cfg.MQTT.Enabled {
		fmt.Println("📡 MQTT уведомления отключены")
		return
	}

	// Используем WebSocket Secure
	broker := fmt.Sprintf("wss://%s:%s", cfg.MQTT.Broker, cfg.MQTT.Port)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID("iserver-monitor")
	opts.SetUsername(cfg.MQTT.Username)
	opts.SetPassword(cfg.MQTT.Password)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetConnectTimeout(30 * time.Second)
	opts.SetCleanSession(true)

	// Отключаем проверку сертификата (если нужно)
	opts.SetTLSConfig(&tls.Config{
		InsecureSkipVerify: true,
	})

	// Обработчики событий
	opts.OnConnect = func(c mqtt.Client) {
		fmt.Println("📡 MQTT подключен к брокеру (wss://)")
	}

	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		fmt.Printf("❌ MQTT соединение потеряно: %v\n", err)
	}

	// Создаём и подключаем клиент
	mqttClient = mqtt.NewClient(opts)
	token := mqttClient.Connect()
	token.Wait()

	if token.Error() != nil {
		fmt.Printf("❌ Ошибка подключения к MQTT: %v\n", token.Error())
		return
	}

	fmt.Printf("📡 MQTT уведомления запущены (интервал: %d мин, %02d:00-%02d:00)\n",
		cfg.MQTT.Interval, cfg.MQTT.StartHour, cfg.MQTT.EndHour)

	go func() {
		ticker := time.NewTicker(time.Duration(cfg.MQTT.Interval) * time.Minute)
		defer ticker.Stop()

		time.Sleep(30 * time.Second)
		checkAndSendMQTT()

		for {
			select {
			case <-ticker.C:
				checkAndSendMQTT()
			case <-shutdown:
				if mqttClient != nil && mqttClient.IsConnected() {
					mqttClient.Disconnect(250)
				}
				fmt.Println("📡 MQTT уведомления остановлены")
				return
			}
		}
	}()
}

// checkAndSendMQTT проверяет статусы и отправляет уведомления
func checkAndSendMQTT() {
	cfg := GetServersConfig()

	// Проверяем рабочее время (исключаем ночь)
	now := time.Now()
	currentHour := now.Hour()
	if currentHour < cfg.MQTT.StartHour || currentHour >= cfg.MQTT.EndHour {
		return
	}

	status := GetFullStatus()

	for _, server := range status.Servers {
		// Проверка времени (critical)
		if server.TimeStatus == "critical" {
			message := fmt.Sprintf("⚠️ ПРОБЛЕМА ВРЕМЕНИ\nСервер: %s\nРазница: %v",
				server.Name, server.TimeDiff)
			sendMQTTMessage(message)
		}

		// Проверка XML (есть ошибка)
		if server.XmlStatus != "" && strings.Contains(server.XmlStatus, "❌") {
			message := fmt.Sprintf("❌ ПРОБЛЕМА XML\nСервер: %s\nСтатус: %s",
				server.Name, server.XmlStatus)
			sendMQTTMessage(message)
		}
	}
}

// sendMQTTMessage отправляет сообщение в MQTT
func sendMQTTMessage(message string) {
	cfg := GetServersConfig()

	if mqttClient == nil || !mqttClient.IsConnected() {
		log.Printf("❌ MQTT не подключен")
		return
	}

	token := mqttClient.Publish(cfg.MQTT.Topic, 0, false, message)
	token.Wait()

	if token.Error() != nil {
		log.Printf("❌ Ошибка отправки MQTT: %v", token.Error())
	} else {
		log.Printf("📡 Отправлено в MQTT: %s", message)
	}
}
