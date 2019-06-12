package utils

import (
	"crypto/tls"
	"fmt"
	"log/syslog"
	"net"
	"os"
	"time"

	"github.com/pkg/errors"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config/dialer"
)

type syslogTestWrap struct {
	*v3.SyslogConfig
}

func (w *syslogTestWrap) TestReachable(dial dialer.Dialer, includeSendTestLog bool) error {
	//TODO: for udp we can't use cluster dialer now, how to handle in cluster deploy syslog
	syslogTestData := newRFC5424Message(w.Severity, w.Program, w.Token, testMessage)
	if w.Protocol == "udp" {
		conn, err := net.Dial("udp", w.Endpoint)
		if err != nil {
			return errors.Wrapf(err, "couldn't dail udp endpoint %s", w.Endpoint)
		}
		defer conn.Close()

		if includeSendTestLog {
			return writeToUDPConn(syslogTestData, w.Endpoint)
		}
		return nil
	}

	var tlsConfig *tls.Config
	if w.EnableTLS {
		hostName, _, err := net.SplitHostPort(w.Endpoint)
		if err != nil {
			return errors.Wrapf(err, "couldn't parse url %s", w.Endpoint)
		}

		tlsConfig, err = buildTLSConfig(w.Certificate, w.ClientCert, w.ClientKey, "", "", hostName, w.SSLVerify)
		if err != nil {
			return err
		}
	}

	conn, err := newTCPConn(dial, w.Endpoint, tlsConfig, true)
	if err != nil {
		return err
	}
	defer conn.Close()

	if !includeSendTestLog {
		return nil
	}

	if _, err = conn.Write(syslogTestData); err != nil {
		return errors.Wrapf(err, "couldn't write data to syslog %s", w.Endpoint)
	}

	// try read to check whether the server close connect already
	// because can't set read deadline for remote dialer, so if the error is timeout will treat as remote server not close the connection
	if _, err := readDataWithTimeout(conn); err != nil && err != errReadDataTimeout {
		return errors.Wrapf(err, "couldn't read data from syslog %s", w.Endpoint)
	}

	return nil
}

func newRFC5424Message(severityStr, app, token, msg string) []byte {
	if app == "" {
		app = "rancher"
	}

	severity := getSeverity(severityStr)
	syslogVersion := 1
	timestamp := time.Now().Format(time.RFC3339)
	msgID := randHex(6)
	hostname, _ := os.Hostname()

	return []byte(fmt.Sprintf("<%d>%v %v %v %v %v %v [%v] %v\n",
		severity,
		syslogVersion,
		timestamp,
		hostname,
		app,
		os.Getpid(),
		msgID,
		token,
		msg,
	))
}

func getSeverity(severityStr string) syslog.Priority {
	severityMap := map[string]syslog.Priority{
		"emerg":   syslog.LOG_EMERG,
		"alert":   syslog.LOG_ALERT,
		"crit":    syslog.LOG_CRIT,
		"err":     syslog.LOG_ERR,
		"warning": syslog.LOG_WARNING,
		"notice":  syslog.LOG_NOTICE,
		"info":    syslog.LOG_INFO,
		"debug":   syslog.LOG_DEBUG,
	}

	if severity, ok := severityMap[severityStr]; ok {
		return severity
	}

	return syslog.LOG_INFO
}

func GetWrapSeverity(severity string) string {
	// for adapt api and fluentd config
	severityMap := map[string]string{
		"warning": "warn",
	}

	wrapSeverity := severityMap[severity]
	if wrapSeverity == "" {
		return severity
	}

	return wrapSeverity
}
