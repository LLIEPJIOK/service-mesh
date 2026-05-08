package domain

import "fmt"

type Identity struct {
	Namespace      string
	ServiceAccount string
}

func (i Identity) String() string {
	return fmt.Sprintf("%s/%s", i.Namespace, i.ServiceAccount)
}

func (i Identity) DNSName() string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", i.ServiceAccount, i.Namespace)
}
