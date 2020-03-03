package kontainerdrivermetadata

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/rancher/pkg/namespace"
	"github.com/rancher/rancher/pkg/settings"
	v1 "github.com/rancher/types/apis/core/v1"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
)

type MetadataController struct {
	NamespacesLister          v1.NamespaceLister
	SystemImagesLister        v3.RKEK8sSystemImageLister
	SystemImages              v3.RKEK8sSystemImageInterface
	ServiceOptionsLister      v3.RKEK8sServiceOptionLister
	ServiceOptions            v3.RKEK8sServiceOptionInterface
	AddonsLister              v3.RKEAddonLister
	Addons                    v3.RKEAddonInterface
	SettingLister             v3.SettingLister
	Settings                  v3.SettingInterface
	CisConfigLister           v3.CisConfigLister
	CisConfig                 v3.CisConfigInterface
	CisBenchmarkVersionLister v3.CisBenchmarkVersionLister
	CisBenchmarkVersion       v3.CisBenchmarkVersionInterface
	url                       *MetadataURL
}

type MetadataURL struct {
	//http path
	path string
	// branch set if .git path by user
	branch string
	// latestHash, isGit set in parseURL
	latestHash string
	isGit      bool
}

const (
	rkeMetadataConfig = "rke-metadata-config"
	refreshInterval   = "refresh-interval-minutes"
	fileLoc           = "data/data.json"
)

var (
	httpClient = &http.Client{
		Timeout: time.Second * 30,
	}
	dataPath    = filepath.Join("./management-state", "driver-metadata", "rke")
	prevHash    string
	fileMapLock = sync.Mutex{}
	fileMapData = map[string]bool{}
)

func Register(ctx context.Context, management *config.ManagementContext) {
	mgmt := management.Management

	m := &MetadataController{
		SystemImagesLister:        mgmt.RKEK8sSystemImages("").Controller().Lister(),
		SystemImages:              mgmt.RKEK8sSystemImages(""),
		ServiceOptionsLister:      mgmt.RKEK8sServiceOptions("").Controller().Lister(),
		ServiceOptions:            mgmt.RKEK8sServiceOptions(""),
		NamespacesLister:          management.Core.Namespaces("").Controller().Lister(),
		AddonsLister:              mgmt.RKEAddons("").Controller().Lister(),
		Addons:                    mgmt.RKEAddons(""),
		SettingLister:             mgmt.Settings("").Controller().Lister(),
		Settings:                  mgmt.Settings(""),
		CisConfigLister:           mgmt.CisConfigs("").Controller().Lister(),
		CisConfig:                 mgmt.CisConfigs(""),
		CisBenchmarkVersionLister: mgmt.CisBenchmarkVersions("").Controller().Lister(),
		CisBenchmarkVersion:       mgmt.CisBenchmarkVersions(""),
	}

	mgmt.Settings("").AddHandler(ctx, "rke-metadata-handler", m.sync)
}

func (m *MetadataController) sync(key string, setting *v3.Setting) (runtime.Object, error) {
	if setting == nil || (setting.Name != rkeMetadataConfig) {
		return nil, nil
	}

	if _, err := m.NamespacesLister.Get("", namespace.GlobalNamespace); err != nil {
		return nil, fmt.Errorf("failed to get %s namespace", namespace.GlobalNamespace)
	}

	value := setting.Value
	if value == "" {
		value = setting.Default
	}
	settingValues, err := getSettingValues(value)
	if err != nil {
		return nil, fmt.Errorf("error getting setting values: %v", err)
	}

	metadata, err := parseURL(settingValues)
	if err != nil {
		return nil, err
	}
	m.url = metadata

	interval, err := convert.ToNumber(settingValues[refreshInterval])
	if err != nil {
		return nil, fmt.Errorf("invalid number %v", interval)
	}

	if interval > 0 {
		logrus.Infof("Refreshing driverMetadata in %v minutes", interval)
		m.Settings.Controller().EnqueueAfter(setting.Namespace, setting.Name, time.Minute*time.Duration(interval))
	}

	return setting, m.refresh()
}

func (m *MetadataController) refresh() error {
	if !toSync(m.url) {
		logrus.Debugf("driverMetadata: skip sync, hash up to date %v", m.url.latestHash)
		return nil
	}
	if !storeMap(m.url) {
		logrus.Debugf("driverMetadata: already in progress")
		return nil
	}
	defer deleteMap(m.url)
	if err := m.Refresh(m.url); err != nil {
		logrus.Warnf("%v, Fallback to refresh from local file path %v", err, DataJSONLocation)
		return errors.Wrapf(m.createOrUpdateMetadataFromLocal(), "failed to refresh from local file path: %s", DataJSONLocation)
	}
	setFinalPath(m.url)
	return nil
}

func (m *MetadataController) Refresh(url *MetadataURL) error {
	data, err := loadData(url)
	if err != nil {
		return errors.Wrapf(err, "failed to refresh data from upstream %v", url.path)
	}
	logrus.Infof("driverMetadata: refreshing data from upstream %v", url.path)
	return errors.Wrap(m.createOrUpdateMetadata(data), "failed to create or update driverMetadata")
}

func GetURLSettingValue() (*MetadataURL, error) {
	settingValues, err := getSettingValues(settings.RkeMetadataConfig.Get())
	if err != nil {
		return nil, err
	}
	url, err := parseURL(settingValues)
	if err != nil {
		return nil, fmt.Errorf("error parsing url %v %v", url, err)
	}
	return url, nil
}
