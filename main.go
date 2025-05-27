package main

import (
	"encoding/xml"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"strconv"

	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const ApiUrl = "http://10.0.0.218:5000/api/text-to-speech?text=" // voice=en_US-lessac-high

type Speak struct {
	XMLName xml.Name `xml:"speak"`
	Voice   Voice    `xml:"voice"`
}

type Voice struct {
	Name    string  `xml:"name,attr"`
	Prosody Prosody `xml:"prosody"`
}

type Prosody struct {
	Rate string `xml:"rate,attr"`
	Text string `xml:",chardata"`
}

// wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1
// ${EDGE_SPEECH_URL}?ConnectionId=${connectId}&TrustedClientToken=${EDGE_API_TOKEN}`
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow any origin
	},
}

func main() {
	http.HandleFunc("/consumer/speech/synthesize/readaloud/edge/v1", handleWS)

	fmt.Println("Server started on wss://localhost:443")
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
	defer logIfErr(ws.Close())

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
		var speak Speak
		if strings.Contains(msgStr, "Path: speech.config") {
			configReceived = true
		} else if strings.Contains(msgStr, "Path: ssml") {
			ssmlReceived = true

			// Extract text from SSML
			err := xml.Unmarshal([]byte(msgStr), &speak)
			if err != nil {
				panic(err)
			}
			fmt.Println("Voice name:", speak.Voice.Name)
			fmt.Println("Rate:", speak.Voice.Prosody.Rate)
			text = strings.TrimSpace(speak.Voice.Prosody.Text)
			fmt.Println("Text:", text)
		}

		// Print message
		//print(msgStr)
		//print(text)

		if configReceived && ssmlReceived {
			wavURL := ApiUrl + url.QueryEscape(text)
			resp, err := http.Get(wavURL)
			if err != nil {
				log.Println("Error calling TTS API:", err)
				break
			}
			wavData, err := io.ReadAll(resp.Body)
			logIfErr(resp.Body.Close())
			if err != nil {
				log.Println("Error reading TTS response:", err)
				break
			}

			// Convert wav to mp3
			rateF, err := strconv.ParseFloat(speak.Voice.Prosody.Rate, 32)
			if err != nil {
				log.Println("Error parsing rate:", err)
				break
			}

			mp3Data, err := wavToMP3(wavData, rateF)
			if err != nil {
				log.Println("Error converting WAV to MP3:", err)
				break
			}

			reqID := "some-random-or-extracted-request-id" // TODO: This doesn't really matter
			// Construct websocket response with audio data
			logIfErr(sendTurnStart(ws, websocket.TextMessage, reqID))
			logIfErr(sendAudio(ws, mp3Data, reqID))
			logIfErr(sendTurnEnd(ws, websocket.TextMessage, reqID))
			break
		}
	}
}

func wavToMP3(wavData []byte, speed float64) ([]byte, error) {
	if speed < 0.5 {
		speed = 0.5
	} else if speed > 2.0 { // TODO: I think the scale goes up to 3.0, but my CPU can't keep up with that...
		speed = 2.0
	}
	filter := fmt.Sprintf("atempo=%.2f", speed)
	tmpWav, err := os.CreateTemp("", "input-*.wav")
	if err != nil {
		return nil, err
	}
	defer logIfErr(os.Remove(tmpWav.Name()))

	_, err = tmpWav.Write(wavData)
	logIfErr(tmpWav.Close())
	if err != nil {
		return nil, err
	}

	// Create temp file for mp3 output // TODO: just do this in memory
	tmpMP3, err := os.CreateTemp("", "output-*.mp3")
	if err != nil {
		return nil, err
	}
	defer logIfErr(os.Remove(tmpMP3.Name()))
	logIfErr(tmpMP3.Close())

	// Run ffmpeg conversion and set speed
	cmd := exec.Command("ffmpeg", "-y", "-i", tmpWav.Name(), "-filter:a", filter, "-codec:a", "libmp3lame", "-qscale:a", "2", tmpMP3.Name())
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

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

func logIfErr(err error) {
	if err != nil {
		log.Println("Error:", err)
	}
}
