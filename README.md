# TextToShim

Prototype Server to redirect cloud based TTS services to local TTS processing.

Work in progress... (currently URLs are hardcoded for my personal use)

## Why?
Certain applications (such as [Readest](https://github.com/readest/readest)) use libraries such as [Edge TTS](https://github.com/andresayac/edge-tts)
for providing Text to Speech services.

While the voices provided are quite high quality, unfortunately they are generated in the cloud via the Bing Speech API.

This incurs privacy concerns (since all text is sent to Microsoft servers), as well as relies on a good internet connection

## System preparation 
create `speech.platform.bing.com` SSL cert with `mkcert`
https://github.com/FiloSottile/mkcert/

add: `127.0.0.1 speech.platform.bing.com` to hosts file

## Host wyoming-piper tts (API on port 5000)
    docker run -it --rm -v ${PWD}\model:/data -p 10200:10200 -p 5000:5000 rhasspy/wyoming-piper:latest --voice en_US-lessac-high

Available voices: https://github.com/rhasspy/piper/blob/master/VOICES.md
