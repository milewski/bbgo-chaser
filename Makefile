build:
	go run ./cmd/bbgo/bbgo.go build --config bbgo.yaml

clean:
	rm -rf build

.PHONY: build
