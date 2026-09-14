package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// DecodeAudio normalizes an uploaded clip into raw pcm_s16le:
//   - audio/wav: RIFF header walked, sample rate read from the fmt chunk
//   - audio/raw: passthrough (sample rate 16000)
//   - anything else is assumed to be a WAV if the RIFF magic is present,
//     otherwise treated as raw PCM.
func DecodeAudio(contentType string, data []byte) (int, []byte, error) {
	if strings.Contains(contentType, "wav") || bytes.HasPrefix(data, []byte("RIFF")) {
		return DecodeWAV(data)
	}
	if strings.Contains(contentType, "raw") {
		return 16000, data, nil
	}
	return 16000, data, nil
}

// DecodeWAV walks the RIFF chunks and returns the sample rate and raw
// pcm_s16le payload (mono/stereo left as-is; Speechmatics and Whisper both
// handle upsampling/interleaving).
func DecodeWAV(data []byte) (int, []byte, error) {
	if len(data) < 44 || !bytes.Equal(data[:4], []byte("RIFF")) {
		return 0, nil, fmt.Errorf("invalid WAV file")
	}
	sampleRate := 16000
	pcm := []byte(nil)
	i := 12
	for i+8 <= len(data) {
		chunkID := string(data[i : i+4])
		size := int(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		bodyStart := i + 8
		if bodyStart+size > len(data) {
			break
		}
		switch chunkID {
		case "fmt ":
			if size >= 16 {
				sampleRate = int(binary.LittleEndian.Uint32(data[bodyStart+4 : bodyStart+8]))
			}
		case "data":
			pcm = data[bodyStart : bodyStart+size]
		}
		i = bodyStart + size + (size % 2)
	}
	if pcm == nil {
		return 0, nil, fmt.Errorf("no data chunk in WAV")
	}
	return sampleRate, pcm, nil
}