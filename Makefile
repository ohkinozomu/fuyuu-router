proto:
	protoc -I=./pkg/data --go_out=./pkg/data --go_opt=paths=source_relative ./pkg/data/data.proto