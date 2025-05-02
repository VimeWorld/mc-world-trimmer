GOOS=darwin GOARCH=arm64 go build -o mc-world-trimmer-macos-arm64
GOOS=darwin GOARCH=amd64 go build -o mc-world-trimmer-macos-amd64
GOOS=linux GOARCH=arm64 go build -o mc-world-trimmer-linux-arm64
GOOS=linux GOARCH=amd64 go build -o mc-world-trimmer-linux-amd64
GOOS=windows GOARCH=amd64 go build -o mc-world-trimmer.exe
