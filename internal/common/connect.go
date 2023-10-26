package common

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"

	"github.com/eclipse/paho.golang/paho"
)

func MQTTConnect(c CommonConfig) *paho.Connect {
	if c.Username != "" && c.Password != "" {
		return &paho.Connect{
			KeepAlive:    10,
			Username:     c.Username,
			Password:     []byte(c.Password),
			UsernameFlag: true,
			PasswordFlag: true,
		}
	}
	return &paho.Connect{
		KeepAlive: 10,
	}
}

func TCPConnect(c CommonConfig) (net.Conn, error) {
	if c.CAFile != "" {
		caCert, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read the CA cert file: %s", err)
		}

		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
			return nil, fmt.Errorf("failed to append CA cert to the pool")
		}

		conf := tls.Config{
			RootCAs: caCertPool,
		}

		if c.Cert != "" && c.Key != "" {
			cert, err := tls.LoadX509KeyPair(c.Cert, c.Key)
			if err != nil {
				return nil, fmt.Errorf("failed to load the cert file: %s", err)
			}
			conf.Certificates = []tls.Certificate{cert}
		}

		conn, err := tls.Dial("tcp", c.MQTTBroker, &conf)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	conn, err := net.Dial("tcp", c.MQTTBroker)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
