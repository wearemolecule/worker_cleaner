// The kubernetes package is a wrapper around the k8s.io/client-go library.
package kubernetes

import (
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	client *kubernetes.Clientset
}

func NewClient(kubeConfigPath string) (*Client, error) {
	var (
		kubeConfig *rest.Config
		err        error
	)

	// If no config path is given assume we are in the cluster
	if kubeConfigPath == "" {
		kubeConfig, err = rest.InClusterConfig()
	} else {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	}

	if err != nil {
		return nil, errors.Wrap(err, "Failed to connect to kubernetes")
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to create kubernetes client from config")
	}

	return &Client{kubeClient}, nil
}

func (c *Client) ListPods(namespace, labelSelector string) (*v1.PodList, error) {
	return c.client.Core().Pods(namespace).List(v1.ListOptions{LabelSelector: labelSelector})
}
