# ABOUTME: Build targets for the keytun project.
# ABOUTME: Provides build, test, and clean commands.

BINARY := keytun
GO := go

.PHONY: build test clean

build:
	$(GO) build -o $(BINARY) .

test:
	$(GO) test ./... -v

clean:
	rm -f $(BINARY)
