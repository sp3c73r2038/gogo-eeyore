package eeyore

import (
	"log"
	"net"
)

type StatClient struct {
	Endpoint string

	conn net.Conn
}

func (client *StatClient) Send(payload string) {
	client.connect()
	client.conn.Write([]byte(payload))
}

func (client *StatClient) connect() {
	if client.conn != nil {
		return
	}
	conn, err := net.Dial("udp", client.Endpoint)
	if err != nil {
		log.Println(err)
		return
	}
	client.conn = conn
}
