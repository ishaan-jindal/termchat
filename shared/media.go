package shared

import "encoding/binary"

// Binary media frame kinds carried on the /media WebSocket.
const (
	MediaKindAudio byte = 0x01
	MediaKindVideo byte = 0x02 // reserved for the future video stream
)

// Media audio codecs.
const (
	MediaCodecPCM16 byte = 0x00 // signed 16-bit little endian mono
)

const (
	// MediaHeaderLen is the frame header size: kind, codec, voice ID.
	MediaHeaderLen = 6

	AudioSampleRate   = 16000
	AudioChannels     = 1
	AudioChunkSamples = 640 // 40 ms at AudioSampleRate
)

// AudioChunkBytes is the payload size of one capture chunk.
const AudioChunkBytes = AudioChunkSamples * 2

// EncodeAudioFrame builds a binary media frame from a header and payload.
func EncodeAudioFrame(kind, codec byte, voiceID uint32, payload []byte) []byte {
	frame := make([]byte, MediaHeaderLen+len(payload))
	frame[0] = kind
	frame[1] = codec
	binary.BigEndian.PutUint32(frame[2:MediaHeaderLen], voiceID)
	copy(frame[MediaHeaderLen:], payload)

	return frame
}

// ParseMediaFrame splits a media frame, rejecting short frames and unknown kinds.
func ParseMediaFrame(b []byte) (kind, codec byte, voiceID uint32, payload []byte, ok bool) {
	if len(b) < MediaHeaderLen {
		return 0, 0, 0, nil, false
	}

	switch b[0] {
	case MediaKindAudio, MediaKindVideo:
	default:
		return 0, 0, 0, nil, false
	}

	return b[0],
		b[1],
		binary.BigEndian.Uint32(b[2:MediaHeaderLen]),
		b[MediaHeaderLen:],
		true
}
