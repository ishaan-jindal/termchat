package main

import (
	"fmt"
	"unicode"

	"termchat/shared"
)

// nicknameError explains why IsValidNickname rejected a non-empty name.
func nicknameError(nick string) string {
	if len([]rune(nick)) > shared.MaxNicknameLength {
		return fmt.Sprintf("names may be at most %d characters", shared.MaxNicknameLength)
	}

	for _, r := range nick {
		if unicode.IsSpace(r) {
			return "spaces are not allowed"
		}
	}

	return "only printable ASCII characters are allowed"
}
