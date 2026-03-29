# ABOUTME: Build recipes for the keytun project.
# ABOUTME: Provides build, test, and clean commands.

binary := "keytun"

build:
    go build -ldflags="-X github.com/gboston/keytun/cmd.Version=dev" -o {{binary}} .

build-linux:
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X github.com/gboston/keytun/cmd.Version=dev" -o {{binary}} .

test:
    go test ./... -v

clean:
    rm -f {{binary}}

demo:
    bash scripts/record-demo.sh

deploy-relay:
    docker build --platform linux/amd64 -t europe-west1-docker.pkg.dev/keytun-website/keytun/relay:latest .
    docker push europe-west1-docker.pkg.dev/keytun-website/keytun/relay:latest
    gcloud run deploy keytun-relay \
        --project keytun-website \
        --region europe-west1 \
        --image europe-west1-docker.pkg.dev/keytun-website/keytun/relay:latest \
        --port 8080 \
        --allow-unauthenticated \
        --session-affinity \
        --timeout 3600 \
        --min-instances 0 \
        --max-instances 3 \
        --cpu 1 \
        --memory 256Mi
