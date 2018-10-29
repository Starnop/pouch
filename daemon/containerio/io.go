package containerio

import (
	"io"
	"time"

	"github.com/alibaba/pouch/daemon/logger"
	"github.com/alibaba/pouch/daemon/logger/crilog"
	"github.com/alibaba/pouch/pkg/streams"

	"github.com/containerd/containerd/cio"
	"github.com/sirupsen/logrus"
)

var logcopierCloseTimeout = 10 * time.Second

// wrapcio will wrap the DirectIO and IO.
//
// When the task exits, the containerd client will close the wrapcio.
type wrapcio struct {
	cio.IO

	cntrio *IO
}

func (wcio *wrapcio) Wait() {
	wcio.IO.Wait()
	wcio.cntrio.Wait()
}

func (wcio *wrapcio) Close() error {
	wcio.IO.Close()

	return wcio.cntrio.Close()
}

// IO represents the streams and logger.
type IO struct {
	id       string
	useStdin bool
	stream   *streams.Stream

	logdriver logger.LogDriver
	logcopier *logger.LogCopier
	criLog    *crilog.Log
}

// NewIO return IO instance.
func NewIO(id string, withStdin bool) *IO {
	s := streams.NewStream()
	if withStdin {
		s.NewStdinInput()
	} else {
		s.NewDiscardStdinInput()
	}

	return &IO{
		id:       id,
		useStdin: withStdin,
		stream:   s,
	}
}

// Reset reset the logdriver.
func (cntrio *IO) Reset() {
	if err := cntrio.Close(); err != nil {
		logrus.WithError(err).WithField("process", cntrio.id).
			Warnf("failed to close during reset IO")
	}

	if cntrio.useStdin {
		cntrio.stream.NewStdinInput()
	} else {
		cntrio.stream.NewDiscardStdinInput()
	}
	cntrio.logdriver = nil
	cntrio.logcopier = nil
	cntrio.criLog = nil
}

// SetLogDriver sets log driver to the IO.
func (cntrio *IO) SetLogDriver(logdriver logger.LogDriver) {
	cntrio.logdriver = logdriver
}

// Stream is used to export the stream field.
func (cntrio *IO) Stream() *streams.Stream {
	return cntrio.stream
}

// AttachCRILog will create CRILog and register it into stream.
func (cntrio *IO) AttachCRILog(path string, withTerminal bool) error {
	l, err := crilog.New(path, withTerminal)
	if err != nil {
		return err
	}

	// NOTE: it might write the same data into two different files, when
	// AttachCRILog is called for ReopenLog.
	cntrio.stream.AddStdoutWriter(l.Stdout)
	if l.Stderr != nil {
		cntrio.stream.AddStderrWriter(l.Stderr)
	}

	// NOTE: when close the previous crilog, it will evicted from the stream.
	if cntrio.criLog != nil {
		cntrio.criLog.Close()
	}
	cntrio.criLog = l
	return nil
}

// Wait wait for coping-data job.
func (cntrio *IO) Wait() {
	cntrio.stream.Wait()
}

// Close closes the stream and the logger.
func (cntrio *IO) Close() error {
	var lastErr error

	if err := cntrio.stream.Close(); err != nil {
		lastErr = err
	}

	if cntrio.logdriver != nil {
		if cntrio.logcopier != nil {
			waitCh := make(chan struct{})
			go func() {
				defer close(waitCh)
				cntrio.logcopier.Wait()
			}()
			select {
			case <-waitCh:
			case <-time.After(logcopierCloseTimeout):
				logrus.Warnf("logcopier doesn't exit in time")
			}
		}

		if err := cntrio.logdriver.Close(); err != nil {
			lastErr = err
		}
	}

	if cntrio.criLog != nil {
		if err := cntrio.criLog.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// InitContainerIO will start logger and coping data from fifo.
func (cntrio *IO) InitContainerIO(dio *DirectIO) (cio.IO, error) {
	if err := cntrio.startLogging(); err != nil {
		return nil, err
	}

	cntrio.stream.CopyPipes(streams.Pipes{
		Stdin:  dio.Stdin,
		Stdout: dio.Stdout,
		Stderr: dio.Stderr,
	})
	return &wrapcio{IO: dio, cntrio: cntrio}, nil
}

func (cntrio *IO) startLogging() error {
	if cntrio.logdriver == nil {
		return nil
	}

	cntrio.logcopier = logger.NewLogCopier(cntrio.logdriver, map[string]io.Reader{
		"stdout": cntrio.stream.NewStdoutPipe(),
		"stderr": cntrio.stream.NewStderrPipe(),
	})
	cntrio.logcopier.StartCopy()
	return nil
}
