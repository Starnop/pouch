// Code forked from Docker project
package sdnotify

import (
	"errors"
	"net"
	"os"

	"github.com/sirupsen/logrus"
)

var SdNotifyNoSocket = errors.New("No socket")

// SdNotify sends a message to the init daemon. It is common to ignore the error.
func SdNotify(state string) error {
	socketAddr := &net.UnixAddr{
		Name: os.Getenv("NOTIFY_SOCKET"),
		Net:  "unixgram",
	}

	if socketAddr.Name == "" {
		logrus.Errorf("no env named NOTIFY_SOCKET exist for systemd notify")
		return SdNotifyNoSocket
	}

	conn, err := net.DialUnix(socketAddr.Net, nil, socketAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(state))
	return err
}

// notifySystem sends a message to the host when the server is ready to be used
func NotifySystemd() {
	// Tell the init daemon we are accepting requests
	go SdNotify("READY=1")
}
