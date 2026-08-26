package shared

import "testing"

func TestEncodeParseAudioFrameRoundtrip(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5}

	frame := EncodeAudioFrame(MediaKindAudio, MediaCodecPCM16, 0xDEADBEEF, payload)

	kind, codec, voiceID, got, ok := ParseMediaFrame(frame)
	if !ok {
		t.Fatal("ParseMediaFrame rejected a valid frame")
	}

	if kind != MediaKindAudio {
		t.Errorf("kind = %#x, want %#x", kind, MediaKindAudio)
	}

	if codec != MediaCodecPCM16 {
		t.Errorf("codec = %#x, want %#x", codec, MediaCodecPCM16)
	}

	if voiceID != 0xDEADBEEF {
		t.Errorf("voiceID = %#x, want 0xdeadbeef", voiceID)
	}

	if string(got) != string(payload) {
		t.Errorf("payload = %v, want %v", got, payload)
	}
}

func TestEncodeAudioFrameEmptyPayload(t *testing.T) {
	frame := EncodeAudioFrame(MediaKindVideo, 0xFF, 7, nil)

	if len(frame) != MediaHeaderLen {
		t.Fatalf("len(frame) = %d, want %d", len(frame), MediaHeaderLen)
	}

	_, _, voiceID, payload, ok := ParseMediaFrame(frame)
	if !ok {
		t.Fatal("ParseMediaFrame rejected a header-only frame")
	}

	if voiceID != 7 || len(payload) != 0 {
		t.Errorf("voiceID = %d, len(payload) = %d; want 7 and 0", voiceID, len(payload))
	}
}

func TestParseMediaFrameRejects(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0x01},
		{0x01, 0x00, 0x00},
		{0x01, 0x00, 0x00, 0x00, 0x00},       // one byte short of header
		{0x7F, 0x00, 0x00, 0x00, 0x00, 0x01}, // unknown kind
	}

	for i, frame := range cases {
		if _, _, _, _, ok := ParseMediaFrame(frame); ok {
			t.Errorf("case %d: ParseMediaFrame(%v) accepted invalid frame", i, frame)
		}
	}
}

func TestAudioChunkMath(t *testing.T) {
	if AudioChunkBytes != AudioChunkSamples*2 {
		t.Errorf("AudioChunkBytes = %d, want s16le size of one chunk", AudioChunkBytes)
	}

	if AudioSampleRate*AudioChannels != 16000 {
		t.Errorf("unexpected stream format: %d Hz x %d channels", AudioSampleRate, AudioChannels)
	}
}
