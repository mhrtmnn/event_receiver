PROG := event_receiver
PROTO_NAME := nunchuk_update

# Protoc generated file is extra package, GOPATH needs to be adjusted. Env vars are not carried over multiple lines.
all:
	export GOPATH=$$(pwd)/go_pkg:~/go && go build -o $(PROG) $(PROG).go

proto:
	mkdir -p go_pkg/src/nunchuk_proto # make directory for go package
	protoc --go_out=go_pkg/src/nunchuk_proto $(PROTO_NAME).proto

clean:
	rm -rf $(PROG) go_pkg
