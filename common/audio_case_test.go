package common

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func testWAV(sampleRate uint32, frames int) []byte {
	const channels = 1
	const bitsPerSample = 16
	dataSize := frames * channels * (bitsPerSample / 8)
	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, sampleRate)
	byteRate := sampleRate * channels * (bitsPerSample / 8)
	_ = binary.Write(buf, binary.LittleEndian, byteRate)
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels*(bitsPerSample/8)))
	_ = binary.Write(buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(make([]byte, dataSize))
	return buf.Bytes()
}

func TestGetAudioDurationAcceptsMixedCaseExtension(t *testing.T) {
	wav := testWAV(8000, 8000)
	for _, ext := range []string{".wav", ".WAV", ".Wav"} {
		duration, err := GetAudioDuration(context.Background(), bytes.NewReader(wav), ext)
		require.NoError(t, err, "extension %s", ext)
		require.InDelta(t, 1, duration, 0.01, "extension %s", ext)
	}
}
