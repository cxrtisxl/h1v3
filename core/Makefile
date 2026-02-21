.PHONY: build test lint clean cold-start

build:
	go build -o bin/h1v3d ./cmd/h1v3d
	go build -o bin/h1v3ctl ./cmd/h1v3ctl

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -rf bin/

# Usage: make cold-start [preset]
# Example: make cold-start simple_dev.json
PRESET := $(or $(word 2,$(MAKECMDGOALS)),default.json)

cold-start: build
	docker volume rm -f h1v3-data
	docker build -t h1v3 .
	docker run -it --rm \
		-p 8080:8080 \
		-v $(PWD)/config.json:/config.json \
		-v h1v3-data:/data \
		-v $(PWD)/presets/$(PRESET):/data/preset.json:ro \
		h1v3 h1v3d --config /config.json -v

# Prevent Make from treating the preset arg as a target
%:
	@:
