package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"io"

	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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
	var text string

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}

		msgStr := string(msg)
		if strings.Contains(msgStr, "Path: speech.config") {
			configReceived = true
			log.Println("Received config")
		} else if strings.Contains(msgStr, "Path: ssml") {
			ssmlReceived = true
			log.Println("Received SSML")

			// Extract text from SSML (you may want to parse XML here; simplified example)
			start := strings.Index(msgStr, "<prosody")
			end := strings.Index(msgStr, "</prosody>")
			if start != -1 && end != -1 && end > start {
				text = msgStr[start:end]
				// Simplify extraction to inside <prosody> ... </prosody>
				text = strings.TrimSpace(text[strings.Index(text, ">")+1:])
				log.Println("Extracted text to synth:", text)
			}
		}
		print(msgStr)

		if configReceived && ssmlReceived {
			// Fetch wav
			wavURL := "http://127.0.0.1:5000/api/text-to-speech?text=" + url.QueryEscape(text) + "&voice=en_US-ryan-high&speaker=en_US-ryan-high"
			resp, err := http.Get(wavURL)
			if err != nil {
				log.Println("Error calling TTS API:", err)
				break
			}
			wavData, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Println("Error reading TTS response:", err)
				break
			}

			// Convert wav to mp3
			mp3Data, err := wavToMP3(wavData)
			if err != nil {
				log.Println("Error converting WAV to MP3:", err)
				break
			}

			reqID := "some-random-or-extracted-request-id"
			sendTurnStart(ws, websocket.TextMessage, reqID)
			sendAudio(ws, mp3Data, reqID)
			sendTurnEnd(ws, websocket.TextMessage, reqID)
			break
		}
	}
}

func wavToMP3(wavData []byte) ([]byte, error) {
	// Write wav to temp file
	tmpWav, err := os.CreateTemp("", "input-*.wav")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpWav.Name())

	_, err = tmpWav.Write(wavData)
	tmpWav.Close()
	if err != nil {
		return nil, err
	}

	// Create temp file for mp3 output
	tmpMP3, err := os.CreateTemp("", "output-*.mp3")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpMP3.Name())
	tmpMP3.Close()

	// Run ffmpeg conversion
	cmd := exec.Command("ffmpeg", "-y", "-i", tmpWav.Name(), "-codec:a", "libmp3lame", "-qscale:a", "2", tmpMP3.Name())
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	// Read MP3 bytes
	mp3Data, err := os.ReadFile(tmpMP3.Name())
	if err != nil {
		return nil, err
	}

	return mp3Data, nil
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
