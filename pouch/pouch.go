package pouch

import "github.com/alibaba/pouch/client"

// NewPouchClient create a client of pouchd.
func NewPouchClient(host string) (client.CommonAPIClient, error) {
	return client.NewAPIClient(host, client.TLSConfig{})
}
