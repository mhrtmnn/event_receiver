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

	"github.com/go-vgo/robotgo"
	"github.com/golang/protobuf/proto"
	"github.com/grandcat/zeroconf"
)

/***********************************************************************************************************************
* GLOBAL DATATYPES
***********************************************************************************************************************/
type last_mouse struct {
	x, y int
}

/***********************************************************************************************************************
* GLOBAL VATIABLES
***********************************************************************************************************************/
const (
	NO_CHANGE         int    = -1
	BUT_THRESH        int    = 8
	DBG_MODE          bool   = false
	target_iface_name string = "enp8s0"
)

var g_last_coord = last_mouse{0, 0}

/***********************************************************************************************************************
* HELPER FUNCS
***********************************************************************************************************************/
// math lib only works on float64
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func Max(x int, y int) int {
	if x > y {
		return x
	}
	return y
}

// custom error handler
func Fatal(err error, c chan struct{}) {
	log.Println(err)
	close(c) // initiate emergency shutdown
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

/***********************************************************************************************************************
* AVAHI SERVICE
***********************************************************************************************************************/
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
		if DBG_MODE {
			fmt.Printf("Found nw interface %s\n", iface.Name)
		}
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

/***********************************************************************************************************************
* PROTOBUF
***********************************************************************************************************************/
// unpack a packed nunchuk update protobuf
func unpack_proto(buff []byte) (*pb.NunchukUpdate, error) {
	/**
	 * nun_upd is pointer to zero initialized structure.
	 * Returning a ptr to a local variable is legal in go,
	 * an escape analysis is performed to check if the address is
	 * used after the function returns. If so, the variable is
	 * allocated on the heap. (https://golang.org/doc/faq#stack_or_heap)
	 */
	nun_upd := &pb.NunchukUpdate{}

	if err := proto.Unmarshal(buff, nun_upd); err != nil {
		log.Println("Failed to parse the protobuf:", err)
		return nil, err
	}

	if DBG_MODE {
		fmt.Printf(">>> Button:\n>>>   C=%s\n>>>   Z=%s\n", nun_upd.Buttons.ButC, nun_upd.Buttons.ButZ)
		fmt.Printf(">>> Joystick:\n>>>   X=%.0f\n>>>   Y=%.0f\n", nun_upd.Joystick.JoyX, nun_upd.Joystick.JoyY)
		fmt.Println()
	}

	return nun_upd, nil
}

/***********************************************************************************************************************
* HID
***********************************************************************************************************************/
// simulate HID events with data from protobuf
func hid_control(upd *pb.NunchukUpdate) {

	// left mouse button clicks
	if upd.Buttons.ButZ == pb.NunchukUpdate_ButInfo_DOWN {
		robotgo.MouseToggle("down", "left")
	}
	if upd.Buttons.ButZ == pb.NunchukUpdate_ButInfo_UP {
		robotgo.MouseToggle("up", "left")

	}

	// left mouse button clicks
	if upd.Buttons.ButC == pb.NunchukUpdate_ButInfo_DOWN {
		robotgo.MouseToggle("down", "right")
	}
	if upd.Buttons.ButC == pb.NunchukUpdate_ButInfo_UP {
		robotgo.MouseToggle("up", "right")
	}

	// Update joystick position if necessary
	if int(upd.Joystick.JoyX) != NO_CHANGE {
		g_last_coord.x = int(upd.Joystick.JoyX) - 128 // Joy{X,Y} takes values in [0x00, 0xff]
	}
	if int(upd.Joystick.JoyY) != NO_CHANGE {
		g_last_coord.y = int(upd.Joystick.JoyY) - 128
	}

	// remove some noise
	if Abs(g_last_coord.x) < BUT_THRESH {
		g_last_coord.x = 0
	}
	if Abs(g_last_coord.y) < BUT_THRESH {
		g_last_coord.y = 0
	}

	// Mouse Movement is done in separate goroutine
	// in order to be able to keep moving the mouse even if joy{x,y} stays constant
}

func mouse_mover(id int, globalQuit chan struct{}, sig chan int) {
	for {

		// Current mouse coords
		m_x, m_y := robotgo.GetMousePos()

		// Use joy position as coord delta
		m_x += g_last_coord.x
		m_y -= g_last_coord.y // origin is at top left corner of monitor, so invert the y delta

		// make sure new coords are >=0, otherwise the curser will jump to the oppsite screen side
		m_x = Max(m_x, 0)
		m_y = Max(m_y, 0)

		if DBG_MODE {
			fmt.Printf("New position (%d,%d), delta (%d, %d)\n", m_x, m_y, g_last_coord.x, g_last_coord.y)
		}

		// Move Mouse to new position
		robotgo.MoveMouseSmooth(m_x, m_y, 0.0, 1.0)
		time.Sleep(10 * time.Millisecond)

		// clean exit
		select {
		case <-globalQuit:
			log.Printf("Shutdown mouse mover\n")
			sig <- id
			return
		default:
			// pass
		}
	}
}

/***********************************************************************************************************************
* UDP SERVER
***********************************************************************************************************************/
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
			if DBG_MODE {
				log.Printf("Received %d bytes from %v\n", len, addr)
			}

			nun_upd, err := unpack_proto(buffer[:len])
			if err == nil {
				hid_control(nun_upd)
			}
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

/***********************************************************************************************************************
* MAIN
***********************************************************************************************************************/
func main() {
	globalQuit := make(chan struct{})
	sig := make(chan int)

	// array of all parallel running functions
	funcs := make([]func(int, chan struct{}, chan int), 4)
	funcs[0] = heart_beat
	funcs[1] = udp_server
	funcs[2] = register_service
	funcs[3] = mouse_mover

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
