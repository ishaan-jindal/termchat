package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

var (
	mediaTokenTTL = 30 * time.Second
)

const mediaTokenBytes = 16

type mediaTokenEntry struct {
	client    *Client
	expiresAt time.Time
}

var mediaTokens = struct {
	sync.Mutex
	tokens map[string]mediaTokenEntry
}{tokens: map[string]mediaTokenEntry{}}

func resetMediaTokens() {
	mediaTokens.Lock()
	mediaTokens.tokens = map[string]mediaTokenEntry{}
	mediaTokens.Unlock()
}

// issueMediaToken mints a single-use token bound to the client.
func issueMediaToken(client *Client) string {
	buf := make([]byte, mediaTokenBytes)

	if _, err := rand.Read(buf); err != nil {
		logger.Println("issuing media token:", err)
		return ""
	}

	token := hex.EncodeToString(buf)

	mediaTokens.Lock()
	mediaTokens.tokens[token] = mediaTokenEntry{
		client:    client,
		expiresAt: time.Now().Add(mediaTokenTTL),
	}
	mediaTokens.Unlock()

	return token
}

// consumeMediaToken redeems a token exactly once, returning its client or
// nil when the token is unknown, expired or already spent.
func consumeMediaToken(token string) *Client {
	mediaTokens.Lock()
	defer mediaTokens.Unlock()

	entry, ok := mediaTokens.tokens[token]

	if ok {
		delete(mediaTokens.tokens, token)
	}

	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}

	return entry.client
}
