package helm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"

	"github.com/rancher/rancher/pkg/controllers/management/compose/common"
	hCommon "github.com/rancher/rancher/pkg/controllers/user/helm/common"
	"github.com/rancher/rancher/pkg/ref"
	mgmtv3 "github.com/rancher/types/apis/management.cattle.io/v3"
	v3 "github.com/rancher/types/apis/project.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/rancher/types/user"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	helmTokenPrefix = "helm-token-"
	description     = "token for helm chart deployment"
)

func Register(user *config.UserContext, kubeConfigGetter common.KubeConfigGetter) {
	appClient := user.Management.Project.Apps("")
	stackLifecycle := &Lifecycle{
		KubeConfigGetter:      kubeConfigGetter,
		TokenClient:           user.Management.Management.Tokens(""),
		UserClient:            user.Management.Management.Users(""),
		UserManager:           user.Management.UserManager,
		K8sClient:             user.K8sClient,
		TemplateVersionClient: user.Management.Management.TemplateVersions(""),
		ListenConfigClient:    user.Management.Management.ListenConfigs(""),
		ClusterName:           user.ClusterName,
		TemplateContentClient: user.Management.Management.TemplateContents(""),
		AppRevisionGetter:     user.Management.Project,
	}
	appClient.AddClusterScopedLifecycle("helm-controller", user.ClusterName, stackLifecycle)
}

type Lifecycle struct {
	KubeConfigGetter      common.KubeConfigGetter
	UserManager           user.Manager
	TokenClient           mgmtv3.TokenInterface
	UserClient            mgmtv3.UserInterface
	TemplateVersionClient mgmtv3.TemplateVersionInterface
	K8sClient             kubernetes.Interface
	ListenConfigClient    mgmtv3.ListenConfigInterface
	ClusterName           string
	TemplateContentClient mgmtv3.TemplateContentInterface
	AppRevisionGetter     v3.AppRevisionsGetter
}

func (l *Lifecycle) Create(obj *v3.App) (*v3.App, error) {
	return obj, nil
}

func (l *Lifecycle) Updated(obj *v3.App) (*v3.App, error) {
	_, projectName := ref.Parse(obj.Spec.ProjectName)
	appRevisionClient := l.AppRevisionGetter.AppRevisions(projectName)
	if obj.Spec.AppRevisionName != "" {
		currentRevision, err := appRevisionClient.Get(obj.Spec.AppRevisionName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if obj.Spec.ExternalID != "" {
			if currentRevision.Status.ExternalID == obj.Spec.ExternalID && reflect.DeepEqual(currentRevision.Status.Answers, obj.Spec.Answers) {
				return obj, nil
			}
		}
		if obj.Status.AppliedFiles != nil {
			if reflect.DeepEqual(obj.Status.AppliedFiles, obj.Spec.Files) && reflect.DeepEqual(currentRevision.Status.Answers, obj.Spec.Answers) {
				return obj, nil
			}
		}
	}

	newObj, err := v3.AppConditionInstalled.Do(obj, func() (runtime.Object, error) {
		template, notes, tempDir, err := generateTemplates(obj, l.TemplateVersionClient, l.TemplateContentClient)
		if err != nil {
			return obj, err
		}
		defer os.RemoveAll(tempDir.FullPath)
		if err := l.Run(obj, template, notes, tempDir); err != nil {
			return obj, err
		}
		return obj, nil
	})
	return newObj.(*v3.App), err
}

func (l *Lifecycle) Remove(obj *v3.App) (*v3.App, error) {
	tempDirs, err := createTempDir(obj)
	if err != nil {
		return obj, err
	}
	defer os.RemoveAll(tempDirs.FullPath)
	err = l.writeKubeConfig(obj, tempDirs.KubeConfigFull)
	if err != nil {
		return obj, err
	}
	_, projectName := ref.Parse(obj.Spec.ProjectName)
	appRevisionClient := l.AppRevisionGetter.AppRevisions(projectName)
	if obj.Spec.AppRevisionName != "" {
		currentRevision, err := appRevisionClient.Get(obj.Spec.AppRevisionName, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return obj, err
		}
		if err == nil {
			tc, err := l.TemplateContentClient.Get(currentRevision.Status.Digest, metav1.GetOptions{})
			if err != nil {
				return obj, err
			}
			if err := kubectlDelete(tc.Data, tempDirs.KubeConfigFull, obj.Spec.TargetNamespace); err != nil {
				return obj, err
			}
		}
	}
	revisions, err := appRevisionClient.List(metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", appLabel, obj.Name),
	})
	if err != nil {
		return obj, err
	}
	for _, revision := range revisions.Items {
		if err := appRevisionClient.Delete(revision.Name, &metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return obj, err
		}
	}
	return obj, nil
}

func (l *Lifecycle) Run(obj *v3.App, template, notes string, tempDirs *hCommon.HelmPath) error {
	err := l.writeKubeConfig(obj, tempDirs.KubeConfigFull)
	if err != nil {
		return err
	}
	if err := kubectlApply(template, tempDirs.KubeConfigFull, obj); err != nil {
		if err := l.createAppRevision(obj, template, notes, true); err != nil {
			return err
		}
		return err
	}
	return l.createAppRevision(obj, template, notes, false)
}

func (l *Lifecycle) createAppRevision(obj *v3.App, template, notes string, failed bool) error {
	_, projectName := ref.Parse(obj.Spec.ProjectName)
	appRevisionClient := l.AppRevisionGetter.AppRevisions(projectName)
	release := &v3.AppRevision{}
	release.GenerateName = "apprevision-"
	release.Labels = map[string]string{
		appLabel: obj.Name,
	}
	if failed {
		release.Labels[failedLabel] = "true"
	}
	release.Status.Answers = obj.Spec.Answers
	release.Status.ProjectName = projectName
	release.Status.ExternalID = obj.Spec.ExternalID
	digest := sha256.New()
	digest.Write([]byte(template))
	tag := hex.EncodeToString(digest.Sum(nil))
	if _, err := l.TemplateContentClient.Get(tag, metav1.GetOptions{}); errors.IsNotFound(err) {
		tc := &mgmtv3.TemplateContent{}
		tc.Name = tag
		tc.Data = template
		if _, err := l.TemplateContentClient.Create(tc); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	release.Status.Digest = tag
	createdRevision, err := appRevisionClient.Create(release)
	if err != nil {
		return err
	}
	obj.Spec.AppRevisionName = createdRevision.Name
	obj.Status.Notes = notes
	if obj.Spec.Files != nil {
		obj.Status.AppliedFiles = obj.Spec.Files
	}
	return err
}

func (l *Lifecycle) writeKubeConfig(obj *v3.App, kubePath string) error {
	userID := obj.Annotations["field.cattle.io/creatorId"]
	user, err := l.UserClient.Get(userID, metav1.GetOptions{})
	if err != nil {
		return err
	}
	token, err := l.UserManager.EnsureToken(helmTokenPrefix+user.Name, description, user.Name)
	if err != nil {
		return err
	}

	kubeConfig := l.KubeConfigGetter.KubeConfig(l.ClusterName, token)
	for k := range kubeConfig.Clusters {
		kubeConfig.Clusters[k].InsecureSkipTLSVerify = true
	}

	return clientcmd.WriteToFile(*kubeConfig, kubePath)
}
