package systemtemplate

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"
	"text/template"

	util "github.com/rancher/rancher/pkg/cluster"
	"github.com/rancher/rancher/pkg/settings"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
)

var (
	t = template.Must(template.New("import").Parse(templateSource))
)

type context struct {
	CAChecksum            string
	AgentImage            string
	AuthImage             string
	TokenKey              string
	Token                 string
	URL                   string
	URLPlain              string
	IsWindowsCluster      bool
	PrivateRegistryConfig string
}

func SystemTemplate(resp io.Writer, agentImage, authImage, token, url string, isWindowsCluster bool,
	cluster *v3.Cluster) error {
	d := md5.Sum([]byte(token))
	tokenKey := hex.EncodeToString(d[:])[:7]

	if authImage == "fixed" {
		authImage = settings.AuthImage.Get()
	}

	privateRegistryConfig, err := util.GeneratePrivateRegistryDockerConfig(util.GetPrivateRepo(cluster))
	if err != nil {
		return err
	}

	context := &context{
		CAChecksum:            CAChecksum(),
		AgentImage:            agentImage,
		AuthImage:             authImage,
		TokenKey:              tokenKey,
		Token:                 base64.StdEncoding.EncodeToString([]byte(token)),
		URL:                   base64.StdEncoding.EncodeToString([]byte(url)),
		URLPlain:              url,
		IsWindowsCluster:      isWindowsCluster,
		PrivateRegistryConfig: privateRegistryConfig,
	}

	return t.Execute(resp, context)
}

func CAChecksum() string {
	ca := settings.CACerts.Get()
	if ca != "" {
		if !strings.HasSuffix(ca, "\n") {
			ca += "\n"
		}
		digest := sha256.Sum256([]byte(ca))
		return hex.EncodeToString(digest[:])
	}
	return ""
}
