package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode"

	"termchat/shared"
)

// promptNickname asks for a chat name until a valid one is entered. Blank
// input falls back to saved when it validates, otherwise to "anonymous".
func promptNickname(in *bufio.Reader, out io.Writer, saved string) string {
	def := ""
	if shared.IsValidNickname(saved) {
		def = saved
	}

	prompt := func() {
		if def != "" {
			fmt.Fprintf(out, "Nickname [%s]: ", def)

			return
		}

		fmt.Fprint(out, "Nickname: ")
	}

	prompt()

	for {
		nick, _ := in.ReadString('\n')
		nick = strings.TrimSpace(nick)

		if nick == "" {
			if def != "" {
				return def
			}

			return "anonymous"
		}

		if !shared.IsValidNickname(nick) {
			fmt.Fprintf(out, "  Invalid nickname %q: %s\n", nick, nicknameError(nick))
			prompt()
			continue
		}

		return nick
	}
}

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
