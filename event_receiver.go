package main

import (
	"fmt"
	"log"
	"net"
	pb "nunchuk_proto"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/grandcat/zeroconf"
)

var target_iface_name = "enp8s0"

// custom error handler
func Fatal(err error, c chan struct{}) {
	log.Println(err)
	close(c) // initiate emergency shutdown
}

// Register the event receiver via mDNS (avahi compatible)
func register_service(id int, globalQuit chan struct{}, sig chan int) {
	var target_iface net.Interface
	log.Printf("Starting zeroconf service\n")

	// only advertise on LAN interface (advertise on all nw interfaces, if interface array is empty)
	ifaces, err := net.Interfaces()
	if err != nil {
		Fatal(err, globalQuit)
		return
	}

	for i, iface := range ifaces {
		// fmt.Printf("Found nw interface %s\n", iface.Name)
		if iface.Name == target_iface_name {
			target_iface = iface
			break
		} else if i == len(ifaces)-1 {
			log.Println("Could not find Interface ", target_iface_name)
		}
	}

	// register mDNS Service
	server, err := zeroconf.Register("EventSender_Zeroconf", "_protobuf._udp", "local.", 8888, nil, []net.Interface{target_iface})
	if err != nil {
		Fatal(err, globalQuit)
		return
	}

	<-globalQuit // blocking read from channel

	log.Printf("Shutting Down server\n")
	server.Shutdown()
	sig <- id // signal main that cleanup is complete
}

// unpack a packed nunchuk update protobuf
func unpack_proto(buff []byte) error {
	nun_upd := &pb.NunchukUpdate{}

	if err := proto.Unmarshal(buff, nun_upd); err != nil {
		log.Println("Failed to parse the protobuf:", err)
		return err
	}

	fmt.Printf(">>> Button:\n>>>   C=%s\n>>>   Z=%s\n", nun_upd.Buttons.ButC, nun_upd.Buttons.ButZ)
	fmt.Printf(">>> Joystick:\n>>>   X=%.0f\n>>>   Y=%.0f\n", nun_upd.Joystick.JoyX, nun_upd.Joystick.JoyY)
	fmt.Println()

	return nil
}

// start udp server and listen for incoming packets
func udp_server(id int, globalQuit chan struct{}, sig chan int) {
	log.Printf("Starting Event Listener\n")

	// start listener
	conn, err := net.ListenPacket("udp", ":8888")
	if err != nil {
		Fatal(err, globalQuit)
		return
	}

	// main receive loop, receive udp packets and unpack the contained protobuf
	buffer := make([]byte, 128)
	for {
		// prevent blocking io (does not work with employed worker model)
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		// non blocking read
		len, addr, err := conn.ReadFrom(buffer)
		if err != nil {
			if !strings.HasSuffix(err.Error(), "i/o timeout") {
				log.Println(err)
			}
		} else {
			// output
			log.Printf("Received %d bytes from %v\n", len, addr)

			unpack_proto(buffer[:len])
		}

		// clean exit
		select {
		case <-globalQuit:
			log.Printf("Shutdown udp_server func\n")
			conn.Close()
			sig <- id
			return
		default:
			// pass
		}
	}
}

func heart_beat(id int, globalQuit chan struct{}, sig chan int) {
	log.Printf("Starting Heartbeat\n")

	for {
		log.Printf("Heartbeat\n")
		time.Sleep(2 * time.Second)

		// clean exit
		select {
		case <-globalQuit:
			log.Printf("Shutdown heartbeat func\n")
			sig <- id
			return
		default:
			// pass
		}
	}
}

func main() {
	globalQuit := make(chan struct{})
	sig := make(chan int)

	// array of all parallel running functions
	funcs := make([]func(int, chan struct{}, chan int), 3)
	funcs[0] = heart_beat
	funcs[1] = udp_server
	funcs[2] = register_service

	for i, f := range funcs {
		// start concurrent functions
		go f(i, globalQuit, sig)
	}

	// prepare signalling for clean exit
	sig_chan := make(chan os.Signal)                       // new channel for elements of type 'os.Signal'
	signal.Notify(sig_chan, os.Interrupt, syscall.SIGTERM) // relay incoming SIGTERM signals to channel 'sig_chan'

	select { // blocking read
	case os_sig := <-sig_chan:
		// exit due to os signal
		log.Printf("Received signal '%s'\n", os_sig.String())
		close(globalQuit) // close is broadcasted to all listeners on the channel

		// wait for all workers to shutdown
		for range funcs {
			id := <-sig
			log.Printf("Worker with id %d finished\n", id)
		}

		log.Printf("All done\n")

	case <-globalQuit:
		// exit due to error in some worker
		log.Printf("Error in some worker, emergency shutdown\n")

		// wait for all non-failing workers to shutdown
		for i := 0; i < len(funcs)-1; i++ {
			select {
			case id := <-sig:
				log.Printf("Worker with id %d finished\n", id)
			case <-time.After(time.Second * 10):
				log.Printf("Timeout, stop waiting\n")
			}
		}
	}
}
