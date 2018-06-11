package mapper

import (
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/norman/types/values"
)

type ContainerSecurityContext struct {
}

func (n ContainerSecurityContext) FromInternal(data map[string]interface{}) {
}

func (n ContainerSecurityContext) ToInternal(data map[string]interface{}) {
	if v, ok := values.GetValue(data, "securityContext"); ok && v != nil {
		sc, err := convert.EncodeToMap(v)
		if err != nil {
			return
		}
		if len(sc) > 2 {
			return
		}
		found := false
		if v, ok := values.GetValue(sc, "capAdd"); ok && v != nil {
			capAdd := convert.ToStringSlice(v)
			if len(capAdd) == 0 {
				found = true
			}
		}
		if found {
			found = false
		} else {
			return
		}

		if v, ok := values.GetValue(sc, "capDrop"); ok && v != nil {
			capAdd := convert.ToStringSlice(v)
			if len(capAdd) == 0 {
				found = true
			}
		}
		if found {
			values.RemoveValue(data, "securityContext")
		}
	}
}

func (n ContainerSecurityContext) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	return nil
}
