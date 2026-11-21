package rpc

// +sand:rpc
type Discovery interface {
	List(node, target string) []string
}

type localDiscovery struct {
}

func (d *localDiscovery) List(node, target string) []string {
	return nil
}

var LocalDiscovery Discovery = &localDiscovery{}
