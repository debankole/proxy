package proxy

import (
	"net/url"
	"testing"
)

func TestServerPool_NextHttpServer(t *testing.T) {
	httpUrls := []*url.URL{
		{Host: "http://server1.com"},
		{Host: "http://server2.com"},
		{Host: "http://server3.com"},
	}
	pool := NewServerPool(httpUrls, nil)

	expectedServers := map[string]struct{}{
		"http://server1.com": {},
		"http://server2.com": {},
		"http://server3.com": {},
	}
	l := len(expectedServers)
	for i := 0; i < l; i++ {
		server := pool.NextHttpServer()

		if _, ok := expectedServers[server.Host]; !ok {
			t.Errorf("Serve %s is not expected", server.Host)
		}

		delete(expectedServers, server.Host)
	}

	if len(expectedServers) != 0 {
		t.Errorf("Not all servers were used")
	}
}

func TestServerPool_NextHttpsServer(t *testing.T) {
	httpsUrls := []*url.URL{
		{Host: "https://server1.com"},
		{Host: "https://server2.com"},
		{Host: "https://server3.com"},
	}
	pool := NewServerPool(httpsUrls, nil)

	expectedServers := map[string]struct{}{
		"https://server1.com": {},
		"https://server2.com": {},
		"https://server3.com": {},
	}

	l := len(expectedServers)
	for i := 0; i < l; i++ {
		server := pool.NextHttpServer()

		if _, ok := expectedServers[server.Host]; !ok {
			t.Errorf("Serve %s is not expected", server.Host)
		}
		delete(expectedServers, server.Host)
	}

	if len(expectedServers) != 0 {
		t.Errorf("Not all servers were used")
	}
}
