PREFIX    ?= /usr/local
BINDIR    ?= $(PREFIX)/bin
MANDIR    ?= $(PREFIX)/share/man/man1
LICENSEDIR?= $(PREFIX)/share/licenses/termchat

VERSION   ?= $(shell git describe --tags --match 'cli-v*' 2>/dev/null | sed 's/^cli-//' || echo dev)
API_URL   ?= https://termchat.sacred99.online
WS_URL    ?= wss://termchat.sacred99.online/ws

LDFLAGS   := -s -w \
             -X main.Version=$(VERSION) \
             -X main.DefaultAPI=$(API_URL) \
             -X main.DefaultWS=$(WS_URL)

.PHONY: build install uninstall clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/termchat ./cli

install:
	install -Dm755 dist/termchat   $(DESTDIR)$(BINDIR)/termchat
	install -Dm644 doc/termchat.1  $(DESTDIR)$(MANDIR)/termchat.1
	install -Dm644 LICENSE         $(DESTDIR)$(LICENSEDIR)/LICENSE

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/termchat
	rm -f $(DESTDIR)$(MANDIR)/termchat.1
	rm -rf $(DESTDIR)$(LICENSEDIR)

clean:
	rm -rf dist/termchat
