package main

import "testing"

func TestValidateTLSConfig(t *testing.T) {
	tests := []struct {
		name         string
		certFile     string
		keyFile      string
		clientCAFile string
		wantErr      bool
	}{
		{name: "plain http - all empty", wantErr: false},
		{name: "direct tls - cert and key", certFile: "c.pem", keyFile: "k.pem", wantErr: false},
		{name: "mtls - cert key and ca", certFile: "c.pem", keyFile: "k.pem", clientCAFile: "ca.pem", wantErr: false},
		{name: "cert without key", certFile: "c.pem", wantErr: true},
		{name: "key without cert", keyFile: "k.pem", wantErr: true},
		{name: "client ca without cert and key", clientCAFile: "ca.pem", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTLSConfig(tt.certFile, tt.keyFile, tt.clientCAFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
