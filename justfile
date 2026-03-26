# ABOUTME: Build recipes for the keytun project.
# ABOUTME: Provides build, test, and clean commands.

binary := "keytun"

build:
    go build -o {{binary}} .

test:
    go test ./... -v

clean:
    rm -f {{binary}}
