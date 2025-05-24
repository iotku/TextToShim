package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

//TIP <p>To run your code, right-click the code and select <b>Run</b>.</p> <p>Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.</p>

// wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1
// ${EDGE_SPEECH_URL}?ConnectionId=${connectId}&TrustedClientToken=${EDGE_API_TOKEN}`
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow any origin
	},
}

func main() {
	http.HandleFunc("/consumer/speech/synthesize/readaloud/edge/v1", handleWS)

	fmt.Println("Server started on ws://localhost:8080")
	err := http.ListenAndServeTLS(":443", "speech.platform.bing.com.pem", "speech.platform.bing.com-key.pem", nil)
	if err != nil {
		log.Fatal("ListenAndServeTLS error:", err)
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	connId := r.URL.Query().Get("ConnectionId")

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer ws.Close()

	fmt.Println("Client connected:", connId)

	var configReceived, ssmlReceived bool

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}

		msgStr := string(msg)
		if strings.Contains(msgStr, "Path: speech.config") {
			configReceived = true
			fmt.Println("Received config")
		} else if strings.Contains(msgStr, "Path: ssml") {
			ssmlReceived = true
			fmt.Println("Received SSML")
		}
		print(msgStr)

		data, err := os.ReadFile("dummy.mp3")
		if err != nil {
			log.Println("Failed to read dummy MP3:", err)
			return
		}
		dummyMP3Bytes := data

		if configReceived && ssmlReceived {
			reqID := "some-random-or-extracted-request-id"
			sendTurnStart(ws, websocket.TextMessage, reqID)
			sendAudio(ws, dummyMP3Bytes, reqID)
			sendTurnEnd(ws, websocket.TextMessage, reqID)
			break
		}
	}
}

func sendTurnStart(ws *websocket.Conn, msgType int, reqID string) error {
	msg := fmt.Sprintf(
		"Path: turn.start\r\nX-RequestId: %s\r\nX-Timestamp: %s\r\n\r\n{}",
		reqID, time.Now().Format(time.RFC1123))
	return ws.WriteMessage(msgType, []byte(msg))
}

func sendAudio(ws *websocket.Conn, mp3data []byte, reqID string) error {
	// Compose header string
	header := fmt.Sprintf(
		"Path: audio\r\nContent-Type: audio/mpeg\r\nX-RequestId: %s\r\nX-Timestamp: %s\r\n\r\n",
		reqID, time.Now().Format(time.RFC1123))

	headerBytes := []byte(header)
	headerLen := uint16(len(headerBytes))

	buf := make([]byte, 2+len(headerBytes)+len(mp3data))
	// Write header length as 2-byte big endian
	buf[0] = byte(headerLen >> 8)
	buf[1] = byte(headerLen & 0xff)
	copy(buf[2:], headerBytes)
	copy(buf[2+len(headerBytes):], mp3data)

	return ws.WriteMessage(websocket.BinaryMessage, buf)
}

func sendTurnEnd(ws *websocket.Conn, msgType int, reqID string) error {
	msg := fmt.Sprintf(
		"Path: turn.end\r\nX-RequestId: %s\r\nX-Timestamp: %s\r\n\r\n{}",
		reqID, time.Now().Format(time.RFC1123))
	return ws.WriteMessage(msgType, []byte(msg))
}
