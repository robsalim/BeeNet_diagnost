package main

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// TCPClient - структура клиента
type TCPClient struct {
	Conn     net.Conn
	Addr     string
	Port     int
	LastSeen time.Time
	Mu       sync.Mutex
}

var (
	tcpClients   = make(map[int]*TCPClient)
	tcpClientsMu sync.RWMutex
)

// StartTCPServer запускает TCP сервер на указанном порту
func StartTCPServer(port int) {
    addr := fmt.Sprintf(":%d", port)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        fmt.Printf("❌ Не удалось запустить сервер на порту %d: %v\n", port, err)
        return
    }
    
    fmt.Printf("🔌 TCP сервер запущен на порту %d\n", port)
    
    for {
        conn, err := listener.Accept()
        if err != nil {
            return
        }
        
        addr := conn.RemoteAddr().String()
        fmt.Printf("📡 MOXA подключилась к порту %d: %s\n", port, addr)
        
        client := &TCPClient{
            Conn:     conn,
            Addr:     addr,
            Port:     port,
            LastSeen: time.Now(),
        }
        
        tcpClientsMu.Lock()
        tcpClients[port] = client  // просто сохраняем, не закрывая старое
        tcpClientsMu.Unlock()
    }
}

// GetTCPClientByPort возвращает активного клиента по порту
func GetTCPClientByPort(port int) *TCPClient {
	tcpClientsMu.RLock()
	defer tcpClientsMu.RUnlock()
	return tcpClients[port]
}

// RemoveTCPClient удаляет клиента
func RemoveTCPClient(port int) {
	tcpClientsMu.Lock()
	defer tcpClientsMu.Unlock()
	if client, exists := tcpClients[port]; exists {
		client.Conn.Close()
		delete(tcpClients, port)
		fmt.Printf("❌ MOXA на порту %d отключена\n", port)
	}
}