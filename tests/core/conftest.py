import os
import pytest
import requests
import time
import urllib3
import yaml
import socket
import subprocess
import json
import rancher
from sys import platform
from .common import random_str
from kubernetes.client import ApiClient, Configuration, CustomObjectsApi
from kubernetes.client.rest import ApiException
from kubernetes.config.kube_config import KubeConfigLoader
from rancher import ApiError
from .cluster_common import \
    generate_cluster_config, \
    create_cluster, \
    import_cluster


# This stops ssl warnings for unsecure certs
urllib3.disable_warnings()


def get_ip():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        # doesn't even have to be reachable
        s.connect(('10.255.255.255', 1))
        IP = s.getsockname()[0]
    except Exception:
        IP = '127.0.0.1'
    finally:
        s.close()
    return IP


IP = get_ip()
SERVER_URL = 'https://' + IP + ':8443'
BASE_URL = SERVER_URL + '/v3'
AUTH_URL = BASE_URL + '-public/localproviders/local?action=login'
DEFAULT_TIMEOUT = 45


class ManagementContext:
    """Contains a client that is scoped to the managment plane APIs. That is,
    APIs that are not specific to a cluster or project."""

    def __init__(self, client, k8s_client=None, user=None):
        self.client = client
        self.k8s_client = k8s_client
        self.user = user


class ClusterContext:
    """Contains a client that is scoped to a specific cluster. Also contains
    a reference to the ManagementContext used to create cluster client and
    the cluster object itself.
    """

    def __init__(self, management, cluster, client):
        self.management = management
        self.cluster = cluster
        self.client = client


class ProjectContext:
    """Contains a client that is scoped to a newly created project. Also
    contains a reference to the clusterContext used to crete the project and
    the project object itself.
    """

    def __init__(self, cluster_context, project, client):
        self.cluster = cluster_context
        self.project = project
        self.client = client


class DINDContext:
    """Returns a DINDContext for a new RKE cluster for the default global
    admin user."""

    def __init__(
        self, name, admin_mc, cluster, client, cluster_file, kube_file
    ):
        self.name = name
        self.admin_mc = admin_mc
        self.cluster = cluster
        self.client = client
        self.cluster_file = cluster_file
        self.kube_file = kube_file


@pytest.fixture(scope="session")
def admin_mc():
    """Returns a ManagementContext for the default global admin user."""
    r = requests.post(AUTH_URL, json={
        'username': 'admin',
        'password': 'admin',
        'responseType': 'json',
    }, verify=False)
    protect_response(r)
    client = rancher.Client(url=BASE_URL, token=r.json()['token'],
                            verify=False)
    k8s_client = kubernetes_api_client(client, 'local')
    admin = client.list_user(username='admin').data[0]
    return ManagementContext(client, k8s_client, user=admin)


@pytest.fixture
def admin_cc(admin_mc):
    """Returns a ClusterContext for the local cluster for the default global
    admin user."""
    cluster, client = cluster_and_client('local', admin_mc.client)
    return ClusterContext(admin_mc, cluster, client)


def cluster_and_client(cluster_id, mgmt_client):
    cluster = mgmt_client.by_id_cluster(cluster_id)
    url = cluster.links.self + '/schemas'
    client = rancher.Client(url=url,
                            verify=False,
                            token=mgmt_client.token)
    return cluster, client


@pytest.fixture
def admin_pc(request, admin_cc):
    """Returns a ProjectContext for a newly created project in the local
    cluster for the default global admin user. The project will be deleted
    when this fixture is cleaned up."""
    admin = admin_cc.management.client
    p = admin.create_project(name='test-' + random_str(),
                             clusterId=admin_cc.cluster.id)
    p = admin.wait_success(p)
    wait_for_condition("BackingNamespaceCreated", "True",
                       admin_cc.management.client, p)
    assert p.state == 'active'
    request.addfinalizer(lambda: admin_cc.management.client.delete(p))
    url = p.links.self + '/schemas'
    return ProjectContext(admin_cc, p, rancher.Client(url=url,
                                                      verify=False,
                                                      token=admin.token))


@pytest.fixture
def user_mc(user_factory):
    """Returns a ManagementContext for a newly created standard user"""
    return user_factory()


@pytest.fixture
def user_factory(admin_mc, remove_resource):
    """Returns a factory for creating new users which a ManagementContext for
    a newly created standard user is returned.

    This user and globalRoleBinding will be cleaned up automatically by the
    fixture remove_resource.
    """
    def _create_user(globalRoleId='user'):
        admin = admin_mc.client
        username = random_str()
        password = random_str()
        user = admin.create_user(username=username, password=password)
        remove_resource(user)
        grb = admin.create_global_role_binding(
            userId=user.id, globalRoleId=globalRoleId)
        remove_resource(grb)
        response = requests.post(AUTH_URL, json={
            'username': username,
            'password': password,
            'responseType': 'json',
        }, verify=False)
        protect_response(response)
        client = rancher.Client(url=BASE_URL, token=response.json()['token'],
                                verify=False)
        return ManagementContext(client, user=user)

    return _create_user


@pytest.fixture
def admin_cc_client(admin_cc):
    """Returns the client from the default admin's ClusterContext"""
    return admin_cc.client


@pytest.fixture
def admin_pc_client(admin_pc):
    """Returns the client from the default admin's ProjectContext """
    return admin_pc.client


@pytest.fixture(scope="session")
def dind_cc(request, admin_mc):
    # verify platform is linux
    if platform != 'linux':
        raise Exception('rke dind only supported on linux')

    def set_server_url(url):
        admin_mc.client.update_by_id_setting(id='server-url', value=url)

    original_url = admin_mc.client.by_id_setting('server-url').value

    # make sure server-url is set to IP address for dind accessibility
    set_server_url(SERVER_URL)

    # revert server url to original when done
    request.addfinalizer(lambda: set_server_url(original_url))

    # create the cluster & import
    name, config, cluster_file, kube_file = generate_cluster_config(request, 1)
    create_cluster(cluster_file)
    cluster = import_cluster(admin_mc, kube_file, cluster_name=name)

    # delete cluster when done
    request.addfinalizer(lambda: admin_mc.client.delete(cluster))

    # wait for cluster to completely provision
    wait_for_condition("Ready", "True", admin_mc.client, cluster, 120)
    cluster, client = cluster_and_client(cluster.id, admin_mc.client)

    # get ip address of cluster node
    node_name = config['nodes'][0]['address']
    node_inspect = subprocess.check_output('docker inspect rke-dind-' +
                                           node_name, shell=True).decode()
    node_json = json.loads(node_inspect)
    node_ip = node_json[0]['NetworkSettings']['IPAddress']

    # update cluster fqdn with node ip
    admin_mc.client.update_by_id_cluster(
        id=cluster.id,
        name=cluster.name,
        localClusterAuthEndpoint={
            'enabled': True,
            'fqdn': node_ip + ':6443',
            'caCerts': cluster.caCert,
        },
    )
    return DINDContext(
        name, admin_mc, cluster, client, cluster_file, kube_file
    )


def wait_for(callback, timeout=DEFAULT_TIMEOUT, fail_handler=None):
    sleep_time = _sleep_time()
    start = time.time()
    ret = callback()
    while ret is None or ret is False:
        time.sleep(next(sleep_time))
        if time.time() - start > timeout:
            exception_msg = 'Timeout waiting for condition.'
            if fail_handler:
                exception_msg = exception_msg + ' Fail handler message: ' + \
                    fail_handler()
            raise Exception(exception_msg)
        ret = callback()
    return ret


def _sleep_time():
    sleep = 0.01
    while True:
        yield sleep
        sleep *= 2
        if sleep > 1:
            sleep = 1


def wait_until_available(client, obj, timeout=DEFAULT_TIMEOUT):
    start = time.time()
    sleep = 0.01
    while True:
        time.sleep(sleep)
        sleep *= 2
        if sleep > 2:
            sleep = 2
        try:
            obj = client.reload(obj)
        except ApiError as e:
            if e.error.status != 403:
                raise e
        else:
            return obj
        delta = time.time() - start
        if delta > timeout:
            msg = 'Timeout waiting for [{}:{}] for condition after {}' \
                  ' seconds'.format(obj.type, obj.id, delta)
            raise Exception(msg)


@pytest.fixture
def remove_resource(admin_mc, request):
    """Remove a resource after a test finishes even if the test fails."""
    client = admin_mc.client

    def _cleanup(resource):
        def clean():
            try:
                client.delete(resource)
            except ApiError as e:
                if e.error.status != 404:
                    raise e
        request.addfinalizer(clean)

    return _cleanup


def wait_for_condition(condition_type, status, client, obj, timeout=45):
    start = time.time()
    obj = client.reload(obj)
    sleep = 0.01
    while not find_condition(condition_type, status, obj):
        time.sleep(sleep)
        sleep *= 2
        if sleep > 2:
            sleep = 2
        obj = client.reload(obj)
        delta = time.time() - start
        if delta > timeout:
            msg = 'Timeout waiting for [{}:{}] for condition after {}' \
                ' seconds'.format(obj.type, obj.id, delta)
            raise Exception(msg)
    return obj


def wait_until(cb, timeout=DEFAULT_TIMEOUT, backoff=True):
    start_time = time.time()
    interval = 1
    while time.time() < start_time + timeout and cb() is False:
        if backoff:
            interval *= 2
        time.sleep(interval)


def find_condition(condition_type, status, obj):
    if not hasattr(obj, "conditions"):
        return False

    if obj.conditions is None:
        return False

    for condition in obj.conditions:
        if condition.type == condition_type and condition.status == status:
            return True
    return False


def kubernetes_api_client(rancher_client, cluster_name):
    c = rancher_client.by_id_cluster(cluster_name)
    kc = c.generateKubeconfig()
    loader = KubeConfigLoader(config_dict=yaml.load(kc.config))
    client_configuration = type.__call__(Configuration)
    loader.load_and_set(client_configuration)
    k8s_client = ApiClient(configuration=client_configuration)
    return k8s_client


def protect_response(r):
    if r.status_code >= 300:
        message = 'Server responded with {r.status_code}\nbody:\n{r.text}'
        raise ValueError(message)


def create_kubeconfig(request, dind_cc, client):
    # request cluster scoped kubeconfig, permissions may not be synced yet
    def generateKubeconfig(max_attempts=5):
        for attempt in range(1, max_attempts+1):
            try:
                # get cluster for client
                cluster = client.by_id_cluster(dind_cc.cluster.id)
                return cluster.generateKubeconfig()['config']
            except ApiError as err:
                if attempt == max_attempts:
                    raise err
            time.sleep(1)

    cluster_kubeconfig = generateKubeconfig()

    # write cluster scoped kubeconfig
    cluster_kubeconfig_file = "kubeconfig-" + random_str() + ".yml"
    f = open(cluster_kubeconfig_file, "w")
    f.write(cluster_kubeconfig)
    f.close()

    # cleanup file when done
    request.addfinalizer(lambda: os.remove(cluster_kubeconfig_file))

    # extract token name
    config = yaml.safe_load(cluster_kubeconfig)
    token_name = config['users'][0]['user']['token'].split(':')[0]

    # wait for token to sync
    crd_client = CustomObjectsApi(
        kubernetes_api_client(
            dind_cc.admin_mc.client,
            dind_cc.cluster.id
        )
    )

    def cluster_token_available():
        try:
            return crd_client.get_namespaced_custom_object(
                'cluster.cattle.io',
                'v3',
                'cattle-system',
                'clusterauthtokens',
                token_name
            )
        except ApiException:
            return None

    wait_for(cluster_token_available)

    return cluster_kubeconfig_file
