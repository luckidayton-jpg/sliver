package rpc

import (
	"net"
	"strconv"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PortInUse used to answer by walking the stored listener records, so a record whose
// socket was gone -- which is what a restart leaves behind -- permanently refused the
// port. Found on a live server: after a redeploy, port 8444 was "in use" with nothing
// listening on it, the console listed zero listeners, and creating one there was
// refused. The operator could not start a C2 at all, with an error naming a free port.
//
// These assert the question it must answer: can the port be taken.

func freePort(t *testing.T) uint16 {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port)
}

// A port with nothing on it is available, whatever the database believes.
func TestPortInUse_FreePortIsNotInUse(t *testing.T) {
	p := freePort(t)
	if err := PortInUse(uint32(p)); err != nil {
		t.Errorf("PortInUse(%d) = %v, want nil: nothing is listening on it", p, err)
	}
}

// And the failure this replaced: a stale record must not be able to block a port.
func TestPortInUse_IgnoresStaleRecords(t *testing.T) {
	// There is deliberately no database setup here. PortInUse must not consult the
	// database at all -- that was the bug, and a test with a populated database would
	// only re-assert the old behaviour.
	p := freePort(t)
	if err := PortInUse(uint32(p)); err != nil {
		t.Errorf("PortInUse(%d) = %v with no listener records: the port was refused "+
			"on the strength of a stored record rather than a live socket", p, err)
	}
}

// A port genuinely occupied is still refused, or two listeners would fight over it.
// This is the half that must not regress when making the check more honest.
func TestPortInUse_RefusesAPortThatIsActuallyBound(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	defer l.Close()
	port := uint16(l.Addr().(*net.TCPAddr).Port)

	err = PortInUse(uint32(port))
	if err == nil {
		t.Fatalf("PortInUse(%d) = nil while a socket is bound to it: two listeners "+
			"would be allowed to fight over the same port", port)
	}
	if got := status.Code(err); got != codes.AlreadyExists {
		t.Errorf("status = %v, want AlreadyExists: a caller distinguishes "+
			"'that port is taken' from 'the C2 is broken' by this code", got)
	}
}

// Zero is not a port, and treating it as one produced a bind on ":0" -- which the
// kernel helpfully assigns, so the probe would pass and claim an arbitrary port was
// free.
func TestPortInUse_RefusesPortZero(t *testing.T) {
	err := PortInUse(0)
	if err == nil {
		t.Fatal("PortInUse(0) = nil: port 0 is a request for the kernel to pick one, " +
			"not a port that is free")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("status = %v, want InvalidArgument", got)
	}
}

// The probe must hand the port straight back. A check that leaks the socket it took
// would make the next attempt fail, and would report its own probe as an occupancy.
func TestPortInUse_ReleasesTheProbeSocket(t *testing.T) {
	p := freePort(t)
	if err := PortInUse(uint32(p)); err != nil {
		t.Fatalf("PortInUse(%d) = %v", p, err)
	}
	// The port must be immediately bindable again.
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(int(p)))
	if err != nil {
		t.Fatalf("the probe socket was not released: %v", err)
	}
	_ = l.Close()
}
