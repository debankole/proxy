package proxy

import (
	"net/url"
	"sync/atomic"
)

// ServerPool holds information about backend servers
type ServerPool struct {
	httpServers  []*url.URL
	httpsServers []*url.URL
	currentHttp  uint64
	currentHttps uint64
}

// NextHttpServer returns the next http server to use in round-robin fashion
func (p *ServerPool) NextHttpServer() *url.URL {
	server := p.httpServers[atomic.AddUint64(&p.currentHttp, 1)%uint64(len(p.httpServers))]
	return server
}

// NextHttpsServer returns the next https server to use in round-robin fashion
func (p *ServerPool) NextHttpsServer() *url.URL {
	server := p.httpsServers[atomic.AddUint64(&p.currentHttps, 1)%uint64(len(p.httpsServers))]
	return server
}

// NewServerPool creates a new ServerPool
func NewServerPool(httpUrls []*url.URL, httpsUrls []*url.URL) *ServerPool {
	pool := &ServerPool{}
	pool.httpServers = httpUrls
	pool.httpsServers = httpsUrls
	return pool
}
