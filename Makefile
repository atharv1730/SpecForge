.PHONY: build test vet lint clean

build:
	go build -o specforge .

test:
	go test ./...

vet:
	go vet ./...

lint: vet

clean:
	rm -f specforge SpecForge
	rm -rf out/
