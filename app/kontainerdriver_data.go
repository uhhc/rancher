package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/rancher/pkg/controllers/management/drivers/kontainerdriver"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func addKontainerDrivers(management *config.ManagementContext) error {
	// create binary drop location if not exists
	err := os.MkdirAll(kontainerdriver.DriverDir, 0777)
	if err != nil {
		return fmt.Errorf("error creating binary drop folder: %v", err)
	}

	creator := driverCreator{
		driversLister: management.Management.KontainerDrivers("").Controller().Lister(),
		drivers:       management.Management.KontainerDrivers(""),
	}

	if err := cleanupImportDriver(creator); err != nil {
		return err
	}

	if err := creator.add("rancherKubernetesEngine"); err != nil {
		return err
	}

	if err := creator.add("googleKubernetesEngine"); err != nil {
		return err
	}

	if err := creator.add("azureKubernetesService"); err != nil {
		return err
	}

	if err := creator.add("amazonElasticContainerService"); err != nil {
		return err
	}

	if err := creator.addCustomDriver(
		"baiducloudcontainerengine",
		"https://github.com/cnrancher/kontainer-engine-driver-baidu/releases/download/v0.2.0/kontainer-engine-driver-baidu-linux",
		"4613e3be3ae5487b0e21dfa761b95de2144f80f98bf76847411e5fcada343d5e",
		"https://cluster-driver.oss-cn-shenzhen.aliyuncs.com/baidu/ui/component.js",
		false,
		"*.aliyuncs.com", "*.baidubce.com",
	); err != nil {
		return err
	}

	if err := creator.addCustomDriver(
		"aliyunkubernetescontainerservice",
		"https://github.com/rancher/kontainer-engine-driver-aliyun/releases/download/v0.2.5/kontainer-engine-driver-aliyun-linux",
		"31aa0a44450c5a5eb128dd0956292dfd91aab726d1a548f6d527a9212a27db9b",
		"",
		false,
		"*.aliyuncs.com",
	); err != nil {
		return err
	}

	if err := creator.addCustomDriver(
		"tencentkubernetesengine",
		"https://github.com/rancher/kontainer-engine-driver-tencent/releases/download/v0.2.3/kontainer-engine-driver-tencent-linux",
		"144f785473290ee2f63cf35da0c6bde12bc307878078500a47a0a8d04422ae53",
		"",
		false,
		"*.tencentcloudapi.com", "*.qcloud.com",
	); err != nil {
		return err
	}

	if err := creator.addCustomDriver(
		"huaweicontainercloudengine",
		"https://github.com/rancher/kontainer-engine-driver-huawei/releases/download/v0.1.2/kontainer-engine-driver-huawei-linux",
		"0b6c1dfaa477a60a3bd9f8a60a55fcafd883866c2c5c387aec75b95d6ba81d45",
		"",
		false,
		"*.myhuaweicloud.com",
	); err != nil {
		return err
	}

	return nil
}

func cleanupImportDriver(creator driverCreator) error {
	var err error
	if _, err = creator.driversLister.Get("", "import"); err == nil {
		err = creator.drivers.Delete("import", &v1.DeleteOptions{})
	}

	if !errors.IsNotFound(err) {
		return err
	}

	return nil
}

type driverCreator struct {
	driversLister v3.KontainerDriverLister
	drivers       v3.KontainerDriverInterface
}

func (c *driverCreator) add(name string) error {
	logrus.Infof("adding kontainer driver %v", name)

	driver, err := c.driversLister.Get("", name)
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = c.drivers.Create(&v3.KontainerDriver{
				ObjectMeta: v1.ObjectMeta{
					Name:      strings.ToLower(name),
					Namespace: "",
				},
				Spec: v3.KontainerDriverSpec{
					URL:     "",
					BuiltIn: true,
					Active:  true,
				},
				Status: v3.KontainerDriverStatus{
					DisplayName: name,
				},
			})
			if err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("error creating driver: %v", err)
			}
		} else {
			return fmt.Errorf("error getting driver: %v", err)
		}
	} else {
		driver.Spec.URL = ""

		_, err = c.drivers.Update(driver)
		if err != nil {
			return fmt.Errorf("error updating driver: %v", err)
		}
	}

	return nil
}

func (c *driverCreator) addCustomDriver(name, url, checksum, uiURL string, active bool, domains ...string) error {
	logrus.Infof("adding kontainer driver %v", name)
	_, err := c.driversLister.Get("", name)
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = c.drivers.Create(&v3.KontainerDriver{
				ObjectMeta: v1.ObjectMeta{
					Name: strings.ToLower(name),
				},
				Spec: v3.KontainerDriverSpec{
					URL:              url,
					BuiltIn:          false,
					Active:           active,
					Checksum:         checksum,
					UIURL:            uiURL,
					WhitelistDomains: domains,
				},
				Status: v3.KontainerDriverStatus{
					DisplayName: name,
				},
			})
			if err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("error creating driver: %v", err)
			}
		} else {
			return fmt.Errorf("error getting driver: %v", err)
		}
	}
	return nil
}
