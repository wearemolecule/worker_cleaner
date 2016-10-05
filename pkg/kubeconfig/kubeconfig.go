package kubeconfig

import (
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/transport"
)

func NewKubernetesConfig(certsPath, serviceHost, servicePort string) (*restclient.Config, error) {
	config, err := restclient.InClusterConfig()
	if err == nil {
		return config, nil
	}

	tlsTransport, err := transport.New(&transport.Config{
		TLS: transport.TLSConfig{
			CAFile:   fmt.Sprintf("%s/%s", certsPath, "ca.pem"),
			CertFile: fmt.Sprintf("%s/%s", certsPath, "cert.pem"),
			KeyFile:  fmt.Sprintf("%s/%s", certsPath, "key.pem"),
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't set up tls transport")
	}

	return &restclient.Config{
		Host:      fmt.Sprintf("https://%s:%s", serviceHost, servicePort),
		Transport: tlsTransport,
	}, nil
}
