BINARY_DIR=bin

all: linux-relay windows-agent-exe

linux-relay:
	mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_DIR)/linux-relay ./cmd/linux-relay

windows-agent-exe:
	mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_DIR)/windows-agent.exe ./cmd/windows-agent

dev-agent-linux:
	mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BINARY_DIR)/dev-agent ./cmd/windows-agent

clean:
	rm -rf $(BINARY_DIR)
