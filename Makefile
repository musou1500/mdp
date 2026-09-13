.PHONY: build build-windows web test dev clean

# The viewer is built first: internal/server embeds internal/server/dist.
build: web
	go build -o mdp .

build-windows: web
	GOOS=windows GOARCH=amd64 go build -o mdp.exe .

web:
	cd web && npm install && npm run build

test:
	go vet ./...
	go test ./...
	cd web && npx tsc --noEmit

dev: build
	./mdp ./testdata/docroot

clean:
	rm -f mdp mdp.exe
