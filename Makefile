build:
	go build -o bin/hexlet-go-crawler ./cmd/hexlet-go-crawler

test:
	go test ./...

run:
ifndef URL
	@echo "Usage: make run URL=https://example.com"
else
	go run ./cmd/hexlet-go-crawler $(URL)
endif