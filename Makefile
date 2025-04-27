export CGO_ENABLED = 0


clean:
	rm -rf ./bin
	rm -rf ./credentials/internal/proto/*.pb.go


generate:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		./credentials/internal/proto/credentials.proto


test: generate
	go test ./credentials/...


test-ci:
	go test -json ./credentials/... \
		| go-ctrf-json-reporter -output ctrf-report.json


build: clean generate
	go build -o="./bin/borg-drone" ./borg-collective/cmd/drone
	go build -o="./bin/cred" ./credentials/cmd/cli
	go build -o="./bin/credstore" ./credentials/cmd/store


build-ci: generate
	go build -o="./bin/borg-drone-$(SUFFIX)" -ldflags="-s -w -X main.version=$(VERSION)" ./borg-collective/cmd/drone
	go build -o="./bin/cred-$(SUFFIX)" -ldflags="-s -w -X main.version=$(VERSION)" ./credentials/cmd/cli
	go build -o="./bin/credstore-$(SUFFIX)" -ldflags="-s -w -X main.version=$(VERSION)" ./credentials/cmd/store

	upx -9 ./bin/* || echo ""
