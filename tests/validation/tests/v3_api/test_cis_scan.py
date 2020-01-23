import pytest
import requests
import time
from rancher import ApiError
from lib.aws import AmazonWebServices
from .common import CLUSTER_MEMBER
from .common import CLUSTER_OWNER
from .common import cluster_cleanup
from .common import get_user_client
from .common import get_user_client_and_cluster
from .common import get_custom_host_registration_cmd
from .common import get_project_client_for_token
from .common import get_client_for_token
from .common import get_cluster_by_name
from .common import if_test_rbac
from .common import PROJECT_OWNER
from .common import PROJECT_MEMBER
from .common import PROJECT_READ_ONLY
from .common import random_test_name
from .common import rbac_get_user_token_by_role
from .common import USER_TOKEN
from .common import validate_cluster_state
from .common import wait_for_cluster_node_count
from .test_rke_cluster_provisioning import HOST_NAME, \
    rke_config_cis, POD_SECURITY_POLICY_TEMPLATE  # NOQA

SECURITY_SCAN_REPORT_PASS_TESTS = 94
SECURITY_SCAN_REPORT_FAIL_TESTS = 3
SECURITY_SCAN_REPORT_TOTAL_TESTS = \
    SECURITY_SCAN_REPORT_PASS_TESTS + SECURITY_SCAN_REPORT_FAIL_TESTS
SECURITY_SCAN_REPORT_REGULAR_PASS_TESTS = 74
SECURITY_SCAN_REPORT_REGULAR_FAIL_TESTS = 23
DEFAULT_TIMEOUT = 120
cluster_detail = {"cluster": None, "nodes": None,
                  "name": None}


def test_cis_scan_run_scan():
    cluster = cluster_detail["cluster"]
    scan_detail = run_scan(cluster, USER_TOKEN)
    report_link = scan_detail["links"]["report"]
    verify_cis_scan_report(report_link, token=USER_TOKEN)


def test_cis_scan_skip_test_ui():
    client = get_user_client()
    cluster = cluster_detail["cluster"]
    # run security scan
    scan_detail = run_scan(cluster, USER_TOKEN)
    report_link = scan_detail["links"]["report"]
    verify_cis_scan_report(report_link, token=USER_TOKEN)

    # get system project
    system_project = cluster.projects(name="System")["data"][0]
    system_project_id = system_project["id"]
    print(system_project)
    p_client = get_project_client_for_token(system_project, USER_TOKEN)

    # check config map is NOT generated for first scan
    try:
        p_client.list_configMap(projectId=system_project_id,
                                namespaceId="security-scan")
    except ApiError as e:
        assert e.error.status == 404, "Config Map is generated for first scan"

    # delete security-scan-cf config if present
    security_scan_config = \
        p_client.list_configMap(projectId=system_project_id,
                                namespaceId="security-scan",
                                id="security-scan:security-scan-cfg",
                                name="security-scan-cfg")
    print(security_scan_config)
    if len(security_scan_config["data"]) != 0:
        p_client.delete(security_scan_config["data"][0])
    # skip action as on UI
    cm_data = {"config.json": "{\"skip\":{\"rke-cis-1.4\":[\"1.1.2\"]}}"}
    p_client.create_configMap(projectId=system_project_id,
                              name="security-scan-cfg",
                              namespaceId="security-scan",
                              id="security-scan:security-scan-cfg",
                              data=cm_data)
    # run security scan
    scan_detail_2 = run_scan(cluster, USER_TOKEN)
    client.reload(scan_detail_2)
    report_link = scan_detail_2["links"]["report"]
    report = verify_cis_scan_report(report_link,
                                    token=USER_TOKEN,
                                    tests_passed=
                                    SECURITY_SCAN_REPORT_PASS_TESTS-1,
                                    tests_skipped=1)
    print(report["results"][0]["checks"][0]["state"])
    assert report["results"][0]["checks"][0]["state"] == "skip", \
        "State of the test is not as expected"

    """As part of clean up
    delete security-scan-cf config
    """
    security_scan_config = \
        p_client.list_configMap(projectId=system_project_id,
                                namespaceId="security-scan",
                                id="security-scan:security-scan-cfg",
                                name="security-scan-cfg")
    print(security_scan_config)
    if len(security_scan_config["data"]) != 0:
        p_client.delete(security_scan_config["data"][0])


def test_cis_scan_skip_test_api():
    client = get_user_client()
    cluster = cluster_detail["cluster"]
    # run security scan
    scan_detail = run_scan(cluster, USER_TOKEN)
    report_link = scan_detail["links"]["report"]
    verify_cis_scan_report(report_link, token=USER_TOKEN)

    # skip test 1.1.3
    cluster.runSecurityScan(overrideSkip=["1.1.3"])
    cluster = client.reload(cluster)
    cluster_scan_report_id = cluster.annotations["field.cattle.io/runCisScan"]
    print(cluster_scan_report_id)
    scan_detail = wait_for_scan_active(cluster_scan_report_id, client)
    wait_for_cis_pod_remove(cluster, cluster_scan_report_id)
    report_link = scan_detail["links"]["report"]
    report = verify_cis_scan_report(report_link,
                                    token=USER_TOKEN,
                                    tests_passed=
                                    SECURITY_SCAN_REPORT_PASS_TESTS-1,
                                    tests_skipped=1)
    assert report["results"][0]["checks"][1]["state"] == "skip", \
        "State of the test is not as expected"


def test_cis_scan_edit_cluster():
    aws_nodes = cluster_detail["nodes"]
    client = get_user_client()
    cluster = cluster_detail["cluster"]
    # Add 2 etcd nodes to the cluster
    for i in range(0, 2):
        aws_node = aws_nodes[3+i]
        aws_node.execute_command("sudo sysctl -w vm.overcommit_memory=1")
        aws_node.execute_command("sudo sysctl -w kernel.panic=10")
        aws_node.execute_command("sudo sysctl -w kernel.panic_on_oops=1")
        docker_run_cmd = get_custom_host_registration_cmd(client,
                                                          cluster,
                                                          ["etcd"],
                                                          aws_node)
        aws_node.execute_command(docker_run_cmd)
    wait_for_cluster_node_count(client, cluster, 5)
    validate_cluster_state(client, cluster, intermediate_state="updating")
    cluster = client.reload(cluster)

    # run CIS Scan
    scan_detail = run_scan(cluster, USER_TOKEN)
    report_link = scan_detail["links"]["report"]
    report = verify_cis_scan_report(report_link,
                                    token=USER_TOKEN,
                                    tests_passed=
                                    SECURITY_SCAN_REPORT_PASS_TESTS-1,
                                    tests_failed=
                                    SECURITY_SCAN_REPORT_FAIL_TESTS+1)
    print(report["results"][3]["checks"][18])
    assert report["results"][3]["checks"][18]["state"] == "mixed"

    # edit nodes and run command
    for i in range(0, 2):
        aws_node = aws_nodes[3+i]
        aws_node.execute_command("sudo useradd etcd")

    # run CIS Scan
    scan_detail = run_scan(cluster, USER_TOKEN)
    report_link = scan_detail["links"]["report"]
    report = verify_cis_scan_report(report_link,
                                    token=USER_TOKEN,
                                    tests_passed=
                                    SECURITY_SCAN_REPORT_PASS_TESTS,
                                    tests_failed=
                                    SECURITY_SCAN_REPORT_FAIL_TESTS)
    print(report["results"][3]["checks"][18]["state"])
    assert report["results"][3]["checks"][18]["state"] == "pass"


@if_test_rbac
def test_rbac_run_scan_cluster_owner():
    client, cluster = get_user_client_and_cluster()
    user_token = rbac_get_user_token_by_role(CLUSTER_OWNER)
    scan_detail = run_scan(cluster, user_token)
    report_link = scan_detail["links"]["report"]
    # failures are 23, because its run on a cluster which is not CIS compliant
    verify_cis_scan_report(report_link,
                           token=USER_TOKEN,
                           tests_passed=
                           SECURITY_SCAN_REPORT_REGULAR_PASS_TESTS,
                           tests_failed=
                           SECURITY_SCAN_REPORT_REGULAR_FAIL_TESTS)


@if_test_rbac
def test_rbac_run_scan_cluster_member():
    client, cluster = get_user_client_and_cluster()
    user_token = rbac_get_user_token_by_role(CLUSTER_MEMBER)
    run_scan(cluster, user_token, can_run_scan=False)


@if_test_rbac
def test_rbac_run_scan_project_owner():
    client, cluster = get_user_client_and_cluster()
    user_token = rbac_get_user_token_by_role(PROJECT_OWNER)
    run_scan(cluster, user_token, can_run_scan=False)


@if_test_rbac
def test_rbac_run_scan_project_member():
    client, cluster = get_user_client_and_cluster()
    user_token = rbac_get_user_token_by_role(PROJECT_MEMBER)
    run_scan(cluster, user_token, can_run_scan=False)


@if_test_rbac
def test_rbac_run_scan_project_read_only():
    client, cluster = get_user_client_and_cluster()
    user_token = rbac_get_user_token_by_role(PROJECT_READ_ONLY)
    run_scan(cluster, user_token, can_run_scan=False)


@pytest.fixture(scope='module', autouse="True")
def create_project_client(request):
    aws_nodes = \
        AmazonWebServices().create_multiple_nodes(
            5, random_test_name(HOST_NAME))
    cluster_detail["nodes"] = aws_nodes
    node_roles = [
        ["controlplane"], ["etcd"], ["worker"]
    ]
    client = get_user_client()
    cluster = client.create_cluster(name=random_test_name(),
                                    driver="rancherKubernetesEngine",
                                    rancherKubernetesEngineConfig=
                                    rke_config_cis,
                                    defaultPodSecurityPolicyTemplateId=
                                    POD_SECURITY_POLICY_TEMPLATE)
    assert cluster.state == "provisioning"
    i = 0
    for i in range(0, 3):
        aws_node = aws_nodes[i]
        aws_node.execute_command("sudo sysctl -w vm.overcommit_memory=1")
        aws_node.execute_command("sudo sysctl -w kernel.panic=10")
        aws_node.execute_command("sudo sysctl -w kernel.panic_on_oops=1")
        if node_roles[i] == ["etcd"]:
            aws_node.execute_command("sudo useradd etcd")
        docker_run_cmd = \
            get_custom_host_registration_cmd(client, cluster, node_roles[i],
                                             aws_node)
        aws_node.execute_command(docker_run_cmd)
    time.sleep(5)
    cluster = validate_cluster_state(client, cluster)

    # the worklods under System project to get active
    time.sleep(10)
    cluster_detail["cluster"] = cluster

    def fin():
        cluster_cleanup(client, cluster, aws_nodes)
    request.addfinalizer(fin)


def verify_cis_scan_report(report_link, token,
                           tests_passed=SECURITY_SCAN_REPORT_PASS_TESTS,
                           tests_failed=SECURITY_SCAN_REPORT_FAIL_TESTS,
                           tests_skipped=0):
    head = {'Authorization': 'Bearer ' + token}
    response = requests.get(report_link, verify=False, headers=head)
    report = response.json()
    assert report["total"] == SECURITY_SCAN_REPORT_TOTAL_TESTS, \
        "Incorrect number of tests run"
    assert report["pass"] == tests_passed, \
        "Incorrect number of tests passed"
    assert report["fail"] == tests_failed, \
        "Incorrect number of failed tests"
    assert report["skip"] == tests_skipped, \
        "Incorrect number of tests skipped"
    return report


def run_scan(cluster, user_token, can_run_scan=True):
    client = get_client_for_token(user_token)
    cluster = get_cluster_by_name(client, cluster.name)
    if can_run_scan:
        cluster.runSecurityScan()
        cluster = client.reload(cluster)
        cluster_scan_report_id = \
            cluster.annotations["field.cattle.io/runCisScan"]
        print(cluster_scan_report_id)
        scan_detail = wait_for_scan_active(cluster_scan_report_id, client)
        wait_for_cis_pod_remove(cluster, cluster_scan_report_id)
        return scan_detail
    else:
        assert "runSecurityScan" not in list(cluster.actions.keys()), \
            "User has Run CIS Scan permission"


def wait_for_scan_active(cluster_scan_report_id,
                         client,
                         timeout=DEFAULT_TIMEOUT):
    scan_detail_data = client.list_clusterScan(name=cluster_scan_report_id)
    scan_detail = scan_detail_data.data[0]
    # wait until scan is active
    start = time.time()
    state_scan = scan_detail["state"]
    while state_scan != "active":
        if time.time() - start > timeout:
            raise AssertionError(
                "Timed out waiting for state of scan report to get to active")
        time.sleep(.5)
        scan_detail_data = client.list_clusterScan(name=cluster_scan_report_id)
        scan_detail = scan_detail_data.data[0]
        state_scan = scan_detail["state"]
        print(state_scan)
    scan_detail_data = client.list_clusterScan(name=cluster_scan_report_id)
    scan_detail = scan_detail_data.data[0]
    return scan_detail


def wait_for_cis_pod_remove(cluster,
                            cluster_scan_report_id,
                            timeout=DEFAULT_TIMEOUT):
    system_project = cluster.projects(name="System")["data"][0]
    p_client = get_project_client_for_token(system_project, USER_TOKEN)
    pod = p_client.list_pod(namespaceId="security-scan",
                            name="security-scan-runner-" +
                                 cluster_scan_report_id)
    start = time.time()
    while len(pod["data"]) != 0:
        if time.time() - start > timeout:
            raise AssertionError(
                "Timed out waiting for removal of security scan pod")
        time.sleep(.5)
        pod = p_client.list_pod(namespaceId="security-scan",
                                name="security-scan-runner-" +
                                     cluster_scan_report_id)
        time.sleep(.5)
