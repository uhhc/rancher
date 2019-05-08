from .common import random_str
from rancher import ApiError

import time
import pytest


def test_workload_image_change_private_registry(admin_pc):
    client = admin_pc.client

    registry1_name = random_str()
    registries = {'index.docker.io': {
                    'username': 'testuser',
                    'password': 'foobarbaz',
                }}
    registry1 = client.create_dockerCredential(name=registry1_name,
                                               registries=registries)
    assert registry1.name == registry1_name

    registry2_name = random_str()
    registries = {'quay.io': {
                    'username': 'testuser',
                    'password': 'foobarbaz',
                }}
    registry2 = client.create_dockerCredential(name=registry2_name,
                                               registries=registries)

    ns = admin_pc.cluster.client.create_namespace(
                                                name=random_str(),
                                                projectId=admin_pc.project.id)
    name = random_str()
    workload = client.create_workload(
                                    name=name,
                                    namespaceId=ns.id,
                                    scale=1,
                                    containers=[{
                                        'name': 'one',
                                        'image': 'testuser/testimage',
                                    }])

    assert workload.name == name
    assert len(workload.imagePullSecrets) == 1
    for secret in workload.imagePullSecrets:
        assert secret['name'] == registry1_name

    containers = [{
                    'name': 'one',
                    'image': 'quay.io/testuser/testimage',
                 }]

    workload = client.update(workload, containers=containers)

    for container in workload.containers:
        assert container['image'] == 'quay.io/testuser/testimage'

    assert len(workload.imagePullSecrets) == 1

    assert workload.imagePullSecrets[0]['name'] == registry2_name

    client.delete(registry1)
    client.delete(registry2)
    client.delete(ns)


def test_workload_ports_change(admin_pc):
    client = admin_pc.client
    ns = admin_pc.cluster.client.create_namespace(name=random_str(),
                                                  projectId=admin_pc.
                                                  project.id)
    # create workload with no ports assigned
    # and verify headless service is created
    name = random_str()
    workload = client.create_workload(
        name=name,
        namespaceId=ns.id,
        scale=1,
        containers=[{
            'name': 'one',
            'image': 'nginx',
        }])
    svc = wait_for_service_create(client, name)
    assert svc.clusterIp is None
    assert svc.name == workload.name
    assert svc.kind == "ClusterIP"

    # update workload with port, and validate cluster ip is set
    ports = [{
        'sourcePort': '0',
        'containerPort': '80',
        'kind': 'ClusterIP',
        'protocol': 'TCP', }]
    client.update(workload,
                  namespaceId=ns.id,
                  scale=1,
                  containers=[{
                      'name': 'one',
                      'image': 'nginx',
                      'ports': ports,
                  }]),
    svc = wait_for_service_cluserip_set(client, name)
    assert svc.clusterIp is not None

    # update workload with no ports, and validate cluster ip is reset
    client.update(workload,
                  namespaceId=ns.id,
                  scale=1,
                  containers=[{
                      'name': 'one',
                      'image': 'nginx',
                      'ports': [],
                  }]),
    svc = wait_for_service_cluserip_reset(client, name)
    assert svc.clusterIp is None

    client.delete(ns)


def test_statefulset_workload_volumemount_subpath(admin_pc):
    client = admin_pc.client
    # setup
    name = random_str()

    # valid volumeMounts
    volumeMounts = [{
         'name': 'vol1',
         'mountPath': 'var/lib/mysql',
         'subPath': 'mysql',
    }]

    containers = [{
         'name': 'mystatefulset',
         'image': 'ubuntu:xenial',
         'volumeMounts': volumeMounts,
    }]

    # invalid volumeMounts
    volumeMounts_one = [{
          'name': 'vol1',
          'mountPath': 'var/lib/mysql',
          'subPath': '/mysql',
    }]

    containers_one = [{
         'name': 'mystatefulset',
         'image': 'ubuntu:xenial',
         'volumeMounts': volumeMounts_one,
    }]

    volumeMounts_two = [{
         'name': 'vol1',
         'mountPath': 'var/lib/mysql',
         'subPath': '../mysql',
    }]

    containers_two = [{
         'name': 'mystatefulset',
         'image': 'ubuntu:xenial',
         'volumeMounts': volumeMounts_two,
    }]

    statefulSetConfig = {
           'podManagementPolicy': 'OrderedReady',
           'revisionHistoryLimit': 10,
           'strategy': 'RollingUpdate',
           'type': 'statefulSetConfig',
    }

    volumes = [{
            'name': 'vol1',
            'persistentVolumeClaim': {
                'persistentVolumeClaimId': "default: myvolume",
                'readOnly': False,
                'type': 'persistentVolumeClaimVolumeSource',
            },
            'type': 'volume',
    }]

    # 1. validate volumeMounts.subPath when workload creating
    # invalid volumeMounts.subPath: absolute path
    with pytest.raises(ApiError) as e:
            client.create_workload(name=name,
                                   namespaceId='default',
                                   scale=1,
                                   containers=containers_one,
                                   statefulSetConfig=statefulSetConfig,
                                   volumes=volumes)

            assert e.value.error.status == 422

    # invalid volumeMounts.subPath: contains '..'
    with pytest.raises(ApiError) as e:
        client.create_workload(name=name,
                               namespaceId='default',
                               scale=1,
                               containers=containers_two,
                               statefulSetConfig=statefulSetConfig,
                               volumes=volumes)

        assert e.value.error.status == 422

    # 2. validate volumeMounts.subPath when workload update
    # create a validate workload then update
    workload = client.create_workload(name=name,
                                      namespaceId='default',
                                      scale=1,
                                      containers=containers,
                                      statefulSetConfig=statefulSetConfig,
                                      volumes=volumes)

    with pytest.raises(ApiError) as e:
        client.update(workload,
                      namespaceId='default',
                      scale=1,
                      containers=containers_one,
                      statefulSetConfig=statefulSetConfig,
                      volumes=volumes)

        assert e.value.error.status == 422

    with pytest.raises(ApiError) as e:
        client.update(workload,
                      namespaceId='default',
                      scale=1,
                      containers=containers_two,
                      statefulSetConfig=statefulSetConfig,
                      volumes=volumes)

        assert e.value.error.status == 422


def wait_for_service_create(client, name, timeout=30):
    start = time.time()
    services = client.list_service(name=name, kind="ClusterIP")
    while len(services) == 0:
        time.sleep(.5)
        services = client.list_service(name=name, kind="ClusterIP")
        if time.time() - start > timeout:
            raise Exception('Timeout waiting for workload service')
    return services.data[0]


def wait_for_service_cluserip_set(client, name, timeout=30):
    start = time.time()
    services = client.list_service(name=name, kind="ClusterIP")
    while len(services) == 0 or services.data[0].clusterIp is None:
        time.sleep(.5)
        services = client.list_service(name=name, kind="ClusterIP")
        if time.time() - start > timeout:
            raise Exception('Timeout waiting for workload service')
    return services.data[0]


def wait_for_service_cluserip_reset(client, name, timeout=30):
    start = time.time()
    services = client.list_service(name=name, kind="ClusterIP")
    while len(services) == 0 or services.data[0].clusterIp is not None:
        time.sleep(.5)
        services = client.list_service(name=name, kind="ClusterIP")
        if time.time() - start > timeout:
            raise Exception('Timeout waiting for workload service')
    return services.data[0]
