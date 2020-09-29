/*
Copyright 2020 Rancher Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by main. DO NOT EDIT.

package v3

import (
	clientset "github.com/uhhc/rancher/pkg/wrangler/generated/clientset/versioned/typed/management.cattle.io/v3"
	informers "github.com/uhhc/rancher/pkg/wrangler/generated/informers/externalversions/management.cattle.io/v3"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/generic"
)

type Interface interface {
	Cluster() ClusterController
	User() UserController
}

func New(controllerManager *generic.ControllerManager, client clientset.ManagementV3Interface,
	informers informers.Interface) Interface {
	return &version{
		controllerManager: controllerManager,
		client:            client,
		informers:         informers,
	}
}

type version struct {
	controllerManager *generic.ControllerManager
	informers         informers.Interface
	client            clientset.ManagementV3Interface
}

func (c *version) Cluster() ClusterController {
	return NewClusterController(v3.SchemeGroupVersion.WithKind("Cluster"), c.controllerManager, c.client, c.informers.Clusters())
}
func (c *version) User() UserController {
	return NewUserController(v3.SchemeGroupVersion.WithKind("User"), c.controllerManager, c.client, c.informers.Users())
}
