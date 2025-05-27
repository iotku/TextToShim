package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"strconv"

	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const ApiUrl = "http://localhost:5000/api/text-to-speech?text=" // voice=en_US-lessac-high

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
	defer func(ws *websocket.Conn) {
		logIfErr(ws.Close(), "Close websocket connection")
	}(ws)

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
			logIfErr(resp.Body.Close(), "TTS Response Body Close")
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
			logIfErr(sendTurnStart(ws, websocket.TextMessage, reqID), "sendTurnStart")
			logIfErr(sendAudio(ws, mp3Data, reqID), "sendAudio")
			logIfErr(sendTurnEnd(ws, websocket.TextMessage, reqID), "sendTurnEnd")
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
	filter := fmt.Sprintf("atempo=%.2f", speed) // TODO: Verify atempo max value is 2.0, could chain filter

	// Run ffmpeg conversion and set speed, pipe:0 (stdin) as input, pipe:1 (stdout) as output
	cmd := exec.Command("ffmpeg", "-y", "-f", "wav", "-i", "pipe:0", "-filter:a", filter, "-codec:a", "libmp3lame", "-qscale:a", "2", "-f", "mp3", "pipe:1")
	stdin, err := cmd.StdinPipe()
	logIfErr(err, "FFmpeg StdinPipe")
	stdout, err := cmd.StdoutPipe()
	logIfErr(err, "FFmpeg StdoutPipe")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Start()
	logIfErr(err, "FFmpeg Start")

	// Write WAV to stdin
	go func() { // don't risk blocking the main thread
		defer func(stdin io.WriteCloser) {
			logIfErr(stdin.Close(), "FFMpeg Close stdin")
		}(stdin)
		_, _ = stdin.Write(wavData)
	}()

	// Read MP3 from stdout
	mp3Data, err := io.ReadAll(stdout)
	logIfErr(err, "Failed to read MP3 data")

	// Wait for the FFmpeg process to finish
	err = cmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w\nstderr: %s", err, stderr.String())
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

func logIfErr(err error, who string) {
	if err != nil {
		log.Println(who+" Error:", err)
	}
}
