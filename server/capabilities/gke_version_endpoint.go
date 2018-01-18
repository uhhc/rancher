package capabilities

import (
	"net/http"
	"os"
	"strings"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"context"
	"google.golang.org/api/container/v1"
	"sync"
	"io/ioutil"
	"io"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
)

const (
	defaultCredentialEnv = "GOOGLE_APPLICATION_CREDENTIALS"
)

var mutex = &sync.Mutex{}

func NewGKECapabilitiesHandler() *gkeVersionHandler {
	return &gkeVersionHandler{}
}

type gkeVersionHandler struct {
}

type errorResponse struct {
	Error string `json:"error"`
}

type requestBody struct {
	Key       string `json:"key"`
	Zone      string `json:"zone"`
	ProjectId string `json:"projectId"`
}

func (g *gkeVersionHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "application/json")

	raw, err := ioutil.ReadAll(req.Body)

	fmt.Println(string(raw))

	var body requestBody
	err = json.Unmarshal(raw, &body)

	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		g.handleErr(writer, fmt.Errorf("cannot parse request body: "+err.Error()))
		return
	}

	key := body.Key
	projectId := body.ProjectId
	zone := body.Zone

	if projectId == "" {
		writer.WriteHeader(http.StatusBadRequest)
		g.handleErr(writer, fmt.Errorf("invalid projectId"))
		return
	}

	if zone == "" {
		writer.WriteHeader(http.StatusBadRequest)
		g.handleErr(writer, fmt.Errorf("invalid zone"))
		return
	}

	if key == "" {
		writer.WriteHeader(http.StatusBadRequest)
		g.handleErr(writer, fmt.Errorf("invalid key"))
		return
	}

	client, err := g.getServiceClient(context.Background(), key)

	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		g.handleErr(writer, err)
		return
	}

	result, err := client.Projects.Zones.GetServerconfig(projectId, zone).Do()

	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		g.handleErr(writer, err)
		return
	}

	serialized, err := json.Marshal(result)

	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		g.handleErr(writer, err)
		return
	}

	writer.Write(serialized)
}

func (g *gkeVersionHandler) handleErr(writer http.ResponseWriter, originalErr error) {
	resp := errorResponse{originalErr.Error()}

	asJson, err := json.Marshal(resp)

	if err != nil {
		logrus.Error("error while marshalling error message '" + originalErr.Error() + "' error was '" + err.Error() + "'")
		writer.Write([]byte(err.Error()))
		return
	}

	writer.Write([]byte(asJson))
}

func (g *gkeVersionHandler) getServiceClient(ctx context.Context, credentialContent string) (*container.Service, error) {
	// The google SDK has no sane way to pass in a TokenSource give all the different types (user, service account, etc)
	// So we actually set an environment variable and then unset it
	mutex.Lock()
	locked := true
	setEnv := false
	cleanup := func() {
		if setEnv {
			os.Unsetenv(defaultCredentialEnv)
		}

		if locked {
			mutex.Unlock()
			locked = false
		}
	}
	defer cleanup()

	file, err := ioutil.TempFile("", "credential-file")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	defer file.Close()

	if _, err := io.Copy(file, strings.NewReader(credentialContent)); err != nil {
		return nil, err
	}

	setEnv = true
	os.Setenv(defaultCredentialEnv, file.Name())

	ts, err := google.DefaultTokenSource(ctx, container.CloudPlatformScope)
	if err != nil {
		return nil, err
	}

	// Unlocks
	cleanup()

	client := oauth2.NewClient(ctx, ts)
	service, err := container.New(client)

	if err != nil {
		return nil, err
	}
	return service, nil
}
