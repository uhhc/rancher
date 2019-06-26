package rkenodeconfigclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"github.com/rancher/rancher/pkg/rkeworker"
	"github.com/sirupsen/logrus"
)

var (
	client = &http.Client{
		Timeout: 300 * time.Second,
	}

	nodeNotFoundRegexp    = regexp.MustCompile(`^node\.management\.cattle\.io.*not found$`)
	clusterNotFoundRegexp = regexp.MustCompile(`^cluster.*not found$`)
)

type ErrNodeOrClusterNotFound struct {
	msg        string
	occursType string
}

func (e *ErrNodeOrClusterNotFound) Error() string {
	return e.msg
}

func (e *ErrNodeOrClusterNotFound) ErrorOccursType() string {
	return e.occursType
}

func newErrNodeOrClusterNotFound(msg, occursType string) *ErrNodeOrClusterNotFound {
	return &ErrNodeOrClusterNotFound{
		msg,
		occursType,
	}
}

func ConfigClient(ctx context.Context, url string, header http.Header, writeCertOnly bool) error {
	// try a few more times because there is a delay after registering a new node
	nodeOrClusterNotFoundRetryLimit := 3
	for {
		nc, err := getConfig(client, url, header)
		if err != nil {
			if _, ok := err.(*ErrNodeOrClusterNotFound); ok {
				if nodeOrClusterNotFoundRetryLimit < 1 {
					// return the error if the node cannot connect to server or remove from a cluster
					return err
				}

				nodeOrClusterNotFoundRetryLimit--
			}

			logrus.Warnf("Error while getting agent config: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if nc != nil {
			logrus.Debugf("Get agent config: %#v", nc)
			return rkeworker.ExecutePlan(ctx, nc, writeCertOnly)
		}

		logrus.Infof("Waiting for node to register. Either cluster is not ready for registering or etcd and controlplane node have to be registered first")
		time.Sleep(2 * time.Second)
	}
}

func getConfig(client *http.Client, url string, header http.Header) (*rkeworker.NodeConfig, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range header {
		req.Header[k] = v
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return &rkeworker.NodeConfig{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := ioutil.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("invalid response %d: %s", resp.StatusCode, string(respBytes))

		if nodeNotFoundRegexp.Match(respBytes) {
			return nil, newErrNodeOrClusterNotFound(errMsg, "node")
		} else if clusterNotFoundRegexp.Match(respBytes) {
			return nil, newErrNodeOrClusterNotFound(errMsg, "cluster")
		}

		return nil, errors.New(errMsg)
	}

	nc := &rkeworker.NodeConfig{}
	return nc, json.NewDecoder(resp.Body).Decode(nc)
}
