package jenkins

import (
	"fmt"
	"github.com/minio/minio-go"
	"github.com/rancher/rancher/pkg/pipeline/utils"
	"github.com/rancher/types/apis/project.cattle.io/v3"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type minioClient struct {
	client minio.Client
}

func (j *Engine) getMinioURL(ns string) (string, error) {
	MinioName := utils.MinioName
	svc, err := j.ServiceLister.Get(ns, MinioName)
	if err != nil {
		return "", err
	}

	ip := svc.Spec.ClusterIP

	return fmt.Sprintf("%s:%d", ip, utils.MinioPort), nil
}

func (j *Engine) getMinioClient(ns string) (*minio.Client, error) {
	url, err := j.getMinioURL(ns)
	if err != nil {
		return nil, err
	}

	user := utils.PipelineSecretDefaultUser
	secret, err := j.SecretLister.Get(ns, utils.PipelineSecretName)
	if err != nil || secret.Data == nil {
		return nil, fmt.Errorf("error get minio token - %v", err)
	}
	token := string(secret.Data[utils.PipelineSecretTokenKey])

	client, err := minio.New(url, user, token, false)
	if err != nil {
		return nil, err
	}
	if j.HTTPClient == nil {
		dial, err := j.Dialer.ClusterDialer(j.ClusterName)
		if err != nil {
			return nil, err
		}

		j.HTTPClient = &http.Client{
			Transport: &http.Transport{
				Dial: dial,
			},
			Timeout: 15 * time.Second,
		}
	}
	client.SetCustomTransport(j.HTTPClient.Transport)

	return client, nil
}

func (j Engine) getStepLogFromMinioStore(execution *v3.PipelineExecution, stage int, step int) (string, error) {
	bucketName := utils.MinioLogBucket
	logName := fmt.Sprintf("%s-%d-%d", execution.Name, stage, step)
	ns := utils.GetPipelineCommonName(execution)
	client, err := j.getMinioClient(ns)
	if err != nil {
		return "", err
	}

	reader, err := client.GetObject(bucketName, logName, minio.GetObjectOptions{})

	//stat, err := reader.Stat()
	if err != nil {
		return "", err
	}

	content, err := ioutil.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (j *Engine) saveStepLogToMinio(execution *v3.PipelineExecution, stage int, step int) error {
	bucketName := utils.MinioLogBucket
	logName := fmt.Sprintf("%s-%d-%d", execution.Name, stage, step)
	ns := utils.GetPipelineCommonName(execution)
	client, err := j.getMinioClient(ns)
	if err != nil {
		return err
	}
	//Make Bucket
	exists, err := client.BucketExists(bucketName)
	if err != nil {
		logrus.Error(err)
	}
	if !exists {
		if err := client.MakeBucket(bucketName, utils.MinioBucketLocation); err != nil {
			return err
		}
	}

	message, err := j.getStepLogFromJenkins(execution, stage, step)
	if err != nil {
		return err
	}

	_, err = client.PutObject(bucketName, logName, strings.NewReader(message), int64(len(message)), minio.PutObjectOptions{})
	return err
}
