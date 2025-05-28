OUTPUT := build/ddcutil-daemon


build: main.go
	go build -o $(OUTPUT) main.go

run: build
	./$(OUTPUT)

.PHONY: run clean

clean:
	rm -rf build
