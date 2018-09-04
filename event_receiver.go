package main

import (
	"fmt"
	"log"
	"net"
	pb "nunchuk_proto"

	"github.com/golang/protobuf/proto"
)

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

func start_server() {

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
	fmt.Printf("Starting Event Receiver\n")

	start_server()
}
