package app

import (
	"github.com/pborman/uuid"
	"github.com/uhhc/rancher/pkg/settings"
)

func addSetting() error {
	return settings.InstallUUID.SetIfUnset(uuid.NewRandom().String())
}
