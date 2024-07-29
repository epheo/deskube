package types

import (
	"github.com/cloudflare/cfssl/config"
)

type GlobalData struct {
	CaKey          []byte
	CaCert         []byte
	IpAddress      string
	ClusterIp      string
	ClusterDNS     string
	ClusterName    string
	ClusterDomain  string
	ClusterNetwork string
	Hostname       string
	ServiceNetwork string
}

type CertData struct {
	CN     string
	O      string
	Hosts  []string
	Config *config.SigningProfile
}

type Service struct {
	Name      string
	User      string
	Server    string
	ExecStart string
}
