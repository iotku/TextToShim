# TextToShim

Prototype Server to redirect cloud based TTS services to local TTS processing.

Work in progress... (currently URLs are hardcoded for my personal use)

## Why?
Certain applications (such as [Readest](https://github.com/readest/readest)) use libraries such as [Edge TTS](https://github.com/andresayac/edge-tts)
for providing Text to Speech services.

While the voices provided are quite high quality, unfortunately they are generated in the cloud via the Bing Speech API.

This incurs privacy concerns (since all text is sent to Microsoft servers), as well as relies on a good internet connection.

## System preparation 

> [!WARNING]
> This will likely break "Read-Aloud" functionality in applications such as Microsoft Edge.
> Remember to remove the CA from `mkcert` and the hosts file entry when not in use.

### Bypass SSL Certificate (!!)
Generate a `speech.platform.bing.com` SSL cert with [mkcert](https://github.com/FiloSottile/mkcert/)
and put the pem files in the current working directory.

### Overwrite Hosts File
Add an entry to system's hosts file:
    
    127.0.0.1 speech.platform.bing.com

### Host wyoming-piper tts (we connect to its API on port 5000)
    docker run -it --rm -v ${PWD}\model:/data -p 10200:10200 -p 5000:5000 rhasspy/wyoming-piper:latest --voice en_US-lessac-high

Available voices: https://github.com/rhasspy/piper/blob/master/VOICES.md

## Running
Runtime Requirements:
    
- ffmpeg


    go mod tidy
    go run .

## TODO
    - Lots of code cleanup
    - Attempt to support more "Read-Aloud" endpoints (e.g. Microsoft Edge)
