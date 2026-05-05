package main

import (
	"fmt"
	"net"
	"sync"
	"time"
)

var (
	tcpServers   = make(map[int]net.Listener)
	tcpServersMu sync.RWMutex
)

// StartTCPServer запускает TCP сервер на указанном порту
func StartTCPServer(port int) {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("❌ Не удалось запустить сервер на порту %d: %v\n", port, err)
		return
	}
	
	tcpServersMu.Lock()
	tcpServers[port] = listener
	tcpServersMu.Unlock()
	
	fmt.Printf("🔌 TCP сервер запущен на порту %d (ожидание подключения счетчиков)\n", port)
	
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go handleTCPConnection(conn, port)
	}
}

// handleTCPConnection обрабатывает подключение счетчика
func handleTCPConnection(conn net.Conn, port int) {
	defer conn.Close()
	
	addr := conn.RemoteAddr().String()
	fmt.Printf("📡 Счетчик подключился к порту %d: %s\n", port, addr)
	
	// Читаем запрос
	buf := make([]byte, 100)
    // Устанавливаем таймаут 8 минут (480 секунд) больше чем на моксе
	conn.SetReadDeadline(time.Now().Add(8 * time.Minute))
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("❌ Ошибка чтения на порту %d: %v\n", port, err)
		return
	}
	
	request := buf[:n]
	fmt.Printf("📥 Получен запрос на порту %d: % X\n", port, request)
	
	// Отправляем ответ эхом
	conn.Write(request)
	fmt.Printf("📤 Отправлен ответ на порту %d: % X\n", port, request)
}