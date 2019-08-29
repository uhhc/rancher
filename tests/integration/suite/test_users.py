import pytest
from kubernetes.client import CustomObjectsApi
from rancher import ApiError
from .conftest import random_str, wait_for


def test_user_cant_delete_self(admin_mc):
    client = admin_mc.client
    with pytest.raises(ApiError) as e:
        client.delete(admin_mc.user)

    assert e.value.error.status == 422


def test_user_cant_deactivate_self(admin_mc):
    client = admin_mc.client
    with pytest.raises(ApiError) as e:
        client.update(admin_mc.user, enabled=False)

    assert e.value.error.status == 422


def test_globalrolebinding_finalizer_cleanup(admin_mc, remove_resource):
    """This ensures that globalrolebinding cleanup of clusters < v2.2.8
        is performed correctly"""
    client = admin_mc.client
    grb = client.create_globalRoleBinding(
        globalRoleId="admin", userId="u-" + random_str()
    )
    remove_resource(grb)
    assert grb.annotations["field.cattle.io/grbUpgrade"] == "true"

    # create a grb without the rancher api with a bad finalizer
    api = CustomObjectsApi(admin_mc.k8s_client)
    json = {
        "apiVersion": "management.cattle.io/v3",
        "globalRoleName": "admin",
        "kind": "GlobalRoleBinding",
        "metadata": {
            "finalizers": ["clusterscoped.controller.cattle.io/grb-sync_fake"],
            "generation": 1,
            "name": "grb-" + random_str(),
        },
        "userName": "u-" + random_str(),
    }
    grb_k8s = api.create_cluster_custom_object(
        group="management.cattle.io",
        version="v3",
        plural="globalrolebindings",
        body=json,
    )
    remove_resource(grb_k8s["metadata"]["name"])
    grb_name = grb_k8s["metadata"]["name"]
    grb_k8s = client.by_id_globalRoleBinding(id=grb_name)

    def check_annotation():
        grb1 = client.by_id_globalRoleBinding(grb_k8s.id)
        try:
            if grb1.annotations["field.cattle.io/grbUpgrade"] == "true":
                return True
            else:
                return False
        except (AttributeError, KeyError):
            return False

    wait_for(check_annotation, fail_handler=lambda: "annotation was not added")
    grb1 = api.get_cluster_custom_object(
        group="management.cattle.io",
        version="v3",
        plural="globalrolebindings",
        name=grb_k8s.id,
    )
    assert (
        "clusterscoped.controller.cattle.io/grb-sync_fake"
        not in grb1["metadata"]["finalizers"]
    )
