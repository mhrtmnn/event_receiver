package main

import (
	"fmt"
	"log"
	"net"
	pb "nunchuk_proto"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang/protobuf/proto"
	"github.com/grandcat/zeroconf"
)

// Register the event receiver via mDNS (avahi compatible)
func register_service() {
	fmt.Printf("Starting zeroconf service\n")

	server, err := zeroconf.Register("EventSender_Zeroconf", "_protobuf._udp", "local.", 8888, nil, nil)
	if err != nil {
		panic(err)
	}
	defer server.Shutdown()

	// Clean exit.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		// Exit by user
	}

	log.Println("Shutting down.")
}

// unpack a packed nunchuk update protobuf
func unpack_proto(buff []byte) error {
	nun_upd := &pb.NunchukUpdate{}

	if err := proto.Unmarshal(buff, nun_upd); err != nil {
		log.Fatalln("Failed to parse the protobuf:", err)
		return err
	}

	fmt.Printf(">>> Button:\n>>>   C=%s\n>>>   Z=%s\n", nun_upd.Buttons.ButC, nun_upd.Buttons.ButZ)
	fmt.Printf(">>> Joystick:\n>>>   X=%.0f\n>>>   Y=%.0f\n", nun_upd.Joystick.JoyX, nun_upd.Joystick.JoyY)
	fmt.Println()

	return nil
}

// start udp server and listen for incoming packets
func start_udp_server() {
	fmt.Printf("Starting Event Listener\n")

	// start listener
	conn, err := net.ListenPacket("udp", ":8888")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	// main receive loop, receive udp packets and unpack the contained protobuf
	buffer := make([]byte, 128)
	for {
		len, addr, err := conn.ReadFrom(buffer)
		if err != nil {
			fmt.Println(err)
			break
		}

		// output
		fmt.Printf("Received %d bytes from %v\n", len, addr)

		unpack_proto(buffer[:len])
	}
}

func main() {
	go start_udp_server()
	register_service()
}
