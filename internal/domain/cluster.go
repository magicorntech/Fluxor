package domain

import (
	"fmt"
	"net/url"
	"regexp"
	"time"
)

type ClusterStatus string

const (
	ClusterStatusUnknown     ClusterStatus = "Unknown"
	ClusterStatusConnected   ClusterStatus = "Connected"
	ClusterStatusUnreachable ClusterStatus = "Unreachable"
)

type Cluster struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	APIServer string            `json:"apiServer"`
	Labels    map[string]string `json:"labels"`
	Status    ClusterStatus     `json:"status"`
	Version   string            `json:"version,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

var nameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Validate checks the user-supplied fields of a cluster.
func (c Cluster) Validate() error {
	if len(c.Name) == 0 || len(c.Name) > 63 || !nameRE.MatchString(c.Name) {
		return fmt.Errorf("name must be 1-63 characters of lowercase letters, digits and '-', starting and ending with a letter or digit")
	}
	u, err := url.Parse(c.APIServer)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("apiServer must be an http(s) URL with a host")
	}
	return nil
}
