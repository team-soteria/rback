rback_version := 0.1

.PHONY: build clean

build :
	GO111MODULE=on GOOS=linux GOARCH=amd64 go build -o ./release/linux_rback main.go
	GO111MODULE=on go build -o ./release/macos_rback main.go 

clean :
	@rm ./release/*