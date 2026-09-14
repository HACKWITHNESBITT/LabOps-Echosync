package audio

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func buildWAV(sampleRate int, pcm []byte) []byte {
	dataSize := len(pcm)
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)      // PCM
	binary.LittleEndian.PutUint16(header[22:24], 1)      // mono
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*2)) // byte rate
	binary.LittleEndian.PutUint16(header[32:34], 2)      // block align
	binary.LittleEndian.PutUint16(header[34:36], 16)     // bits
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))
	return append(header, pcm...)
}

func TestDecodeWAV_ExtractsSampleRateAndPCM(t *testing.T) {
	pcm := []byte{0x00, 0x00, 0xFF, 0x7F}
	data := buildWAV(44100, pcm)
	sr, decoded, err := DecodeWAV(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sr != 44100 {
		t.Errorf("sample rate=%d, want 44100", sr)
	}
	if !bytes.Equal(decoded, pcm) {
		t.Errorf("pcm mismatch")
	}
}

func TestDecodeAudio_RawPassthrough(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x02}
	sr, decoded, err := DecodeAudio("audio/raw", raw)
	if err != nil {
		t.Fatal(err)
	}
	if sr != 16000 {
		t.Errorf("default sample rate=%d, want 16000", sr)
	}
	if !bytes.Equal(decoded, raw) {
		t.Errorf("raw should pass through untouched")
	}
}

func TestDecodeWAV_RejectsGarbage(t *testing.T) {
	if _, _, err := DecodeWAV([]byte("hello")); err == nil {
		t.Fatal("expected error for non-WAV input")
	}
}