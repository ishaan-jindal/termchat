package shared

import "encoding/binary"

// Binary media frame kinds carried on the /media WebSocket.
const (
	MediaKindAudio byte = 0x01
	MediaKindVideo byte = 0x02 // baseline JPEG frame, rendered as ANSI art
)

// Media audio codecs.
const (
	MediaCodecPCM16 byte = 0x00 // signed 16-bit little endian mono
)

// Media video codecs.
const (
	MediaCodecJPEG byte = 0x01 // one baseline JPEG image per frame
)

// Video stream parameters; receivers scale frames to their terminal.
const (
	VideoCapWidth  = 320
	VideoCapHeight = 240
	VideoCapFPS    = 10

	// VideoMaxFrameBytes bounds one encoded frame on the wire.
	VideoMaxFrameBytes = 128 * 1024

	// VideoPixelDepth is the anonymity floor: senders block-average every
	// frame to depth-by-depth pixel blocks before encoding, so the wire
	// only ever carries pixelated video. There is no opt-out.
	VideoPixelDepth = 4
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

// EncodeMediaFrame builds a binary media frame from a header and payload.
// The ID field identifies the sender's stream for every media kind.
func EncodeMediaFrame(kind, codec byte, streamID uint32, payload []byte) []byte {
	frame := make([]byte, MediaHeaderLen+len(payload))
	frame[0] = kind
	frame[1] = codec
	binary.BigEndian.PutUint32(frame[2:MediaHeaderLen], streamID)
	copy(frame[MediaHeaderLen:], payload)

	return frame
}

// EncodeAudioFrame builds a binary media frame for an audio stream.
func EncodeAudioFrame(kind, codec byte, voiceID uint32, payload []byte) []byte {
	return EncodeMediaFrame(kind, codec, voiceID, payload)
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
