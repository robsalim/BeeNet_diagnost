package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmt.Println("======================================")
	fmt.Println("  Dual IServer Monitor v1.0")
	fmt.Println("  Мониторинг двух независимых серверов")
	fmt.Println("======================================")

	// Загружаем конфигурацию серверов
	if err := LoadServersConfig(); err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	if err := LoadModemsConfig(); err != nil {
		log.Printf("Предупреждение: не удалось загрузить modems.json: %v", err)
	}
	
	if err := LoadMetersConfig(); err != nil {
		log.Printf("Предупреждение: не удалось загрузить meters.json: %v", err)
	}
	
	cfg := GetServersConfig()
	
	setupSignalHandler()

	printConfigInfo(cfg)

	startComponents(cfg)

	waitForShutdown()
}

func setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-c
		fmt.Printf("\nПолучен сигнал: %v\n", sig)
		close(shutdown)
	}()
}

func printConfigInfo(cfg ServersConfig) {
	fmt.Println()
	fmt.Println("КОНФИГУРАЦИЯ:")
	fmt.Printf("  Веб-интерфейс: http://localhost%s\n", cfg.WebPort)
	fmt.Printf("  Интервал проверки: %d секунд\n", cfg.CheckIntervalSeconds)
	fmt.Println()
	fmt.Println("МОНИТОРИНГ СЕРВЕРОВ:")
	for i, server := range cfg.Servers {
		fmt.Printf("  Сервер %d: %s (%s)\n", i+1, server.Name, server.Address)
	}
	fmt.Println()
}

func startComponents(cfg ServersConfig) {
	// Запускаем мониторинг
	go StartMonitoring()
	
	//Запуск планировщика
	go StartScheduler()
	
	// Запускаем TCP серверы для всех портов из списка приборов
	go startTCPServersFromMeters()



	// Запускаем веб-сервер
	go func() {
		// Регистрируем обработчики
		http.HandleFunc("/api/full-status", handleFullStatusAPI)
		http.HandleFunc("/api/all-servers", handleAllServersStatusAPI)
		http.HandleFunc("/api/server-status", handleServerStatusAPI)
		http.HandleFunc("/api/db-health", handleDatabaseHealthAPI)
		http.HandleFunc("/api/data-delays", handleDataDelaysAPI)
		http.HandleFunc("/api/privileges", handlePrivilegesAPI)
		http.HandleFunc("/api/restart", handleRestartAPI)
		http.HandleFunc("/api/stop", handleStopAPI)

		http.HandleFunc("/api/modems-tcp-test", handleModemsTCPTestAPI)
		http.HandleFunc("/api/modems-ping-test", handleModemsPingTestAPI)
		http.HandleFunc("/api/ping", handlePingAPI)
		http.HandleFunc("/api/check-meter", handleCheckMeter)
		http.HandleFunc("/api/check-local-port", handleCheckLocalPort)
				
		http.HandleFunc("/api/meters", handleMetersAPI)

		
		http.HandleFunc("/api/doc", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "api.txt")
		})

		// Статические файлы
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "web/index.html")
		})
		http.HandleFunc("/web/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, r.URL.Path[1:])
		})

		fmt.Printf("🌐 Веб-сервер запущен на порту %s\n", cfg.WebPort)
		if err := http.ListenAndServe(cfg.WebPort, nil); err != nil {
			log.Fatalf("Ошибка запуска веб-сервера: %v", err)
		}
	}()

	fmt.Println("✅ Все компоненты запущены!")
	fmt.Println("📡 Ожидание подключений...")
	fmt.Println()
	fmt.Println("Доступные адреса:")
	fmt.Printf("  • http://localhost%s\n", cfg.WebPort)
	fmt.Printf("  • http://127.0.0.1%s\n", cfg.WebPort)

	getNetworkInfo(cfg.WebPort)
}

func waitForShutdown() {
	select {
	case <-shutdown:
		fmt.Println("\n🛑 Получен сигнал завершения...")
		time.Sleep(1 * time.Second)
		fmt.Println("✅ Приложение успешно завершено")
		os.Exit(0)
	}
}

func getNetworkInfo(port string) {
	interfaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("  • (Не удалось получить сетевые интерфейсы)")
		return
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			ip = ip.To4()
			if ip == nil {
				continue
			}

			fmt.Printf("  • http://%s%s\n", ip.String(), port)
		}
	}
}

func startTCPServersFromMeters() {
	meters := GetMetersConfig()
	ports := make(map[int]bool)
	
	// Собираем уникальные порты из всех приборов (только где нет "сервер")
	for _, meter := range meters.Meters {
		connectionPort := meter.ConnectionPort
		
		// Пропускаем, если есть слово "сервер" (это удаленные модемы, не локальные)
		if strings.Contains(strings.ToLower(connectionPort), "сервер") {
			continue
		}
		
		// Извлекаем порт (последние 4 цифры)
		portMatch := regexp.MustCompile(`(\d{4})$`).FindStringSubmatch(connectionPort)
		if len(portMatch) > 0 {
			port, err := strconv.Atoi(portMatch[1])
			if err == nil {
				ports[port] = true
			}
		}
	}
	
	// Запускаем TCP серверы на каждом порту
	for port := range ports {
		go StartTCPServer(port)
		time.Sleep(100 * time.Millisecond)
	}
	
	if len(ports) > 0 {
		fmt.Printf("🔌 Запущено TCP серверов на портах: %v\n", ports)
	} else {
		fmt.Println("🔌 Нет локальных портов для запуска TCP серверов")
	}
}
