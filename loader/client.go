package loader

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"

	"fmt"

	"golang.org/x/net/http2"
)

func client(client *http.Client, clientCert, clientKey, caCert string, usehttp2 bool) (*http.Client, error) {
	if clientCert == "" && clientKey == "" && caCert == "" {
		return client, nil
	}

	if clientCert == "" {
		return nil, fmt.Errorf("client certificate can't be empty")
	}

	if clientKey == "" {
		return nil, fmt.Errorf("client key can't be empty")
	}
	cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, fmt.Errorf("Unable to load cert tried to load %v and %v but got %v", clientCert, clientKey, err)
	}

	// Load our CA certificate
	clientCACert, err := ioutil.ReadFile(caCert)
	if err != nil {
		return nil, fmt.Errorf("Unable to open cert %v", err)
	}

	clientCertPool := x509.NewCertPool()
	clientCertPool.AppendCertsFromPEM(clientCACert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      clientCertPool,
	}

	tlsConfig.BuildNameToCertificate()
	t := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	if usehttp2 {
		http2.ConfigureTransport(t)
	}
	client.Transport = t
	return client, nil
}
