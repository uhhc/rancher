package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rancher/rke/hosts"
	"github.com/rancher/rke/log"
	"github.com/sirupsen/logrus"
)

const (
	HealthzAddress   = "localhost"
	HealthzEndpoint  = "/healthz"
	HTTPProtoPrefix  = "http://"
	HTTPSProtoPrefix = "https://"
)

func runHealthcheck(ctx context.Context, host *hosts.Host, serviceName string, localConnDialerFactory hosts.DialerFactory, url string) error {
	log.Infof(ctx, "[healthcheck] Start Healthcheck on service [%s] on host [%s]", serviceName, host.Address)
	port, err := getPortFromURL(url)
	if err != nil {
		return err
	}
	client, err := getHealthCheckHTTPClient(host, port, localConnDialerFactory)
	if err != nil {
		return fmt.Errorf("Failed to initiate new HTTP client for service [%s] for host [%s]", serviceName, host.Address)
	}
	for retries := 0; retries < 10; retries++ {
		if err = getHealthz(client, serviceName, host.Address, url); err != nil {
			logrus.Debugf("[healthcheck] %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Infof(ctx, "[healthcheck] service [%s] on host [%s] is healthy", serviceName, host.Address)
		return nil
	}
	return fmt.Errorf("Failed to verify healthcheck: %v", err)
}

func getHealthCheckHTTPClient(host *hosts.Host, port int, localConnDialerFactory hosts.DialerFactory) (*http.Client, error) {
	host.LocalConnPort = port
	var factory hosts.DialerFactory
	if localConnDialerFactory == nil {
		factory = hosts.LocalConnFactory
	} else {
		factory = localConnDialerFactory
	}
	dialer, err := factory(host)
	if err != nil {
		return nil, fmt.Errorf("Failed to create a dialer for host [%s]: %v", host.Address, err)
	}
	return &http.Client{
		Transport: &http.Transport{
			Dial:            dialer,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}, nil
}

func getHealthz(client *http.Client, serviceName, hostAddress, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("Failed to check %s for service [%s] on host [%s]: %v", url, serviceName, hostAddress, err)
	}
	if resp.StatusCode != http.StatusOK {
		statusBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("service [%s] is not healthy response code: [%d], response body: %s", serviceName, resp.StatusCode, statusBody)
	}
	return nil
}

func getPortFromURL(url string) (int, error) {
	port := strings.Split(strings.Split(url, ":")[2], "/")[0]
	intPort, err := strconv.Atoi(port)
	if err != nil {
		return 0, err
	}
	return intPort, nil
}
