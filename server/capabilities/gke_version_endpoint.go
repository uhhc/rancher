package capabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/rancher/kontainer-engine/drivers/gke"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/container/v1"
)

const (
	defaultCredentialEnv = "GOOGLE_APPLICATION_CREDENTIALS"
)

func NewGKECapabilitiesHandler() *GKEVersionHandler {
	return &GKEVersionHandler{}
}

type GKEVersionHandler struct {
}

type errorResponse struct {
	Error string `json:"error"`
}

type versionsRequestBody struct {
	Credentials string `json:"credentials"`
	Zone        string `json:"zone"`
	ProjectID   string `json:"projectId"`
}

func (g *gkeVersionHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "application/json")

	raw, err := ioutil.ReadAll(req.Body)

	var body versionsRequestBody
	err = json.Unmarshal(raw, &body)

	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		g.handleErr(writer, fmt.Errorf("cannot parse request body: "+err.Error()))
		return
	}

	credentials := body.Credentials
	projectID := body.ProjectID
	zone := body.Zone

	if projectID == "" {
		writer.WriteHeader(http.StatusBadRequest)
		g.handleErr(writer, fmt.Errorf("invalid projectId"))
		return
	}

	if zone == "" {
		writer.WriteHeader(http.StatusBadRequest)
		g.handleErr(writer, fmt.Errorf("invalid zone"))
		return
	}

	if credentials == "" {
		writer.WriteHeader(http.StatusBadRequest)
		g.handleErr(writer, fmt.Errorf("invalid credentials"))
		return
	}

	client, err := g.getServiceClient(context.Background(), credentials)

	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		g.handleErr(writer, err)
		return
	}

	result, err := client.Projects.Zones.GetServerconfig(projectID, zone).Do()

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

func (g *GKEVersionHandler) handleErr(writer http.ResponseWriter, originalErr error) {
	resp := errorResponse{originalErr.Error()}

	asJSON, err := json.Marshal(resp)

	if err != nil {
		logrus.Error("error while marshalling error message '" + originalErr.Error() + "' error was '" + err.Error() + "'")
		writer.Write([]byte(err.Error()))
		return
	}

	writer.Write([]byte(asJSON))
}

func (g *GKEVersionHandler) getServiceClient(ctx context.Context, credentialContent string) (*container.Service, error) {
	// The google SDK has no sane way to pass in a TokenSource give all the different types (user, service account, etc)
	// So we actually set an environment variable and then unset it
	gke.EnvMutex.Lock()
	locked := true
	setEnv := false
	cleanup := func() {
		if setEnv {
			os.Unsetenv(defaultCredentialEnv)
		}

		if locked {
			gke.EnvMutex.Unlock()
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
