VERSION_BUILD=$(date -u "+%Y-%m-%dT%H:%M:%SZ")
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-X 'main/update.VersionBuild=${VERSION_BUILD}'" -o portable