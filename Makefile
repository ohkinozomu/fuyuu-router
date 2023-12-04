proto:
	protoc -I=./pkg/data --go_out=./pkg/data --go_opt=paths=source_relative ./pkg/data/data.proto

vtproto:
	protoc -I=./pkg/data \
	--go_out=./pkg/data \
	--go_opt=paths=source_relative \
	--go-vtproto_out=./pkg/data \
	--plugin go-vtproto=protoc-gen-go-vtproto \
    --go-vtproto_opt=features=marshal+unmarshal+size \
	--go-vtproto_opt=paths=source_relative \
	./pkg/data/data.proto