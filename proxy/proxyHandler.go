package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var dialer = websocket.DefaultDialer

// Interface for waiting for connections to close
var ActiveConnWaiter ConnWaiter

// WaitGroup for waiting for connections to close
var connWaitGroup *sync.WaitGroup

// ConnWaiter is an interface for waiting for connections to close
type ConnWaiter interface {
	Wait()
}

func init() {
	connWaitGroup = &sync.WaitGroup{}
	ActiveConnWaiter = connWaitGroup
}

// ProxyHandler returns a handler that forwards requests to the next server in the pool
func ProxyHandler(pool *ServerPool, ws bool, https bool, skipCertCheck bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var server *url.URL
		if https {
			server = pool.NextHttpsServer()
		} else {
			server = pool.NextHttpServer()
		}

		if ws {
			proxyWebSocket(server, w, r)
		} else {
			proxy := &httputil.ReverseProxy{
				Rewrite: func(r *httputil.ProxyRequest) {
					r.SetURL(server)
					r.Out.Host = r.In.Host
					r.SetXForwarded()
				},
			}
			if skipCertCheck {
				proxy.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				}
			}
			proxy.ServeHTTP(w, r)
		}
	})
}

// Proxy WebSocket connections
func proxyWebSocket(server *url.URL, rw http.ResponseWriter, req *http.Request) {

	// Copy the headers from the incoming request to the dialer
	requestHeader := http.Header{}
	if origin := req.Header.Get("Origin"); origin != "" {
		requestHeader.Add("Origin", origin)
	}
	for _, prot := range req.Header[http.CanonicalHeaderKey("Sec-WebSocket-Protocol")] {
		requestHeader.Add("Sec-WebSocket-Protocol", prot)
	}
	for _, cookie := range req.Header[http.CanonicalHeaderKey("Cookie")] {
		requestHeader.Add("Cookie", cookie)
	}
	if req.Host != "" {
		requestHeader.Set("Host", req.Host)
	}

	// Adding the client IP to the list of addresses in the X-Forwarded-For (if we are not the first proxy)
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		requestHeader.Set("X-Forwarded-For", clientIP)
	}

	requestHeader.Set("X-Forwarded-Proto", "http")
	if req.TLS != nil {
		requestHeader.Set("X-Forwarded-Proto", "https")
	}

	// Create a connection to the backend server
	urlStr := fmt.Sprintf("ws://%s%s", server.Host, req.URL.Path)
	connToBackend, resp, err := dialer.Dial(urlStr, requestHeader)

	connWaitGroup.Add(1)
	defer connWaitGroup.Done()

	if err != nil {
		log.Printf("Couldn't dial to remote backend '%s' %s", server.String(), err)
		if resp != nil {
			// If response is not nil, copy it to the client
			if err := copyResponse(rw, resp); err != nil {
				log.Printf("Couldn't write response to client after failed remote backend dial: %s", err)
			}
		} else {
			http.Error(rw, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		}
		return
	}
	defer connToBackend.Close()

	// Copy the headers from the Dial handshake to the upgrader
	upgradeHeader := http.Header{}
	if hdr := resp.Header.Get("Sec-Websocket-Protocol"); hdr != "" {
		upgradeHeader.Set("Sec-Websocket-Protocol", hdr)
	}
	if hdr := resp.Header.Get("Set-Cookie"); hdr != "" {
		upgradeHeader.Set("Set-Cookie", hdr)
	}

	// Upgrading the request to a WebSocket connection.
	connToClient, err := upgrader.Upgrade(rw, req, upgradeHeader)

	// Add the connection to the wait group and remove it when the function returns
	// This is used to wait for all connections to close before shutting down the server
	connWaitGroup.Add(1)
	defer connWaitGroup.Done()

	if err != nil {
		log.Printf("Couldn't upgrade %s", err)
		return
	}
	defer connToClient.Close()

	errToClient := make(chan error, 1)
	errToBackend := make(chan error, 1)

	go copyMessages(connToClient, connToBackend, errToClient)
	go copyMessages(connToBackend, connToClient, errToBackend)

	select {
	case err = <-errToClient:
		log.Printf("Error when copying from backend to client: %v", err)
	case err = <-errToBackend:
		log.Printf("Error when copying from client to backend: %v", err)
	}
}

// Copy messages between two WebSocket connections
func copyMessages(dst, src *websocket.Conn, errChan chan error) {
	for {
		msgType, msg, err := src.ReadMessage()
		if err != nil {
			// Send a close message to the destination in case of an error
			m := websocket.FormatCloseMessage(websocket.CloseNormalClosure, fmt.Sprintf("%v", err))
			if e, ok := err.(*websocket.CloseError); ok {
				if e.Code != websocket.CloseNoStatusReceived {
					m = websocket.FormatCloseMessage(e.Code, e.Text)
				}
			}
			dst.WriteMessage(websocket.CloseMessage, m)
			errChan <- err
			break
		}

		err = dst.WriteMessage(msgType, msg)
		if err != nil {
			errChan <- err
			break
		}
	}
}

func copyResponse(rw http.ResponseWriter, resp *http.Response) error {
	rwHeader := rw.Header()
	for key, h := range resp.Header {
		for _, v := range h {
			rwHeader.Add(key, v)
		}
	}
	rw.WriteHeader(resp.StatusCode)
	defer resp.Body.Close()

	_, err := io.Copy(rw, resp.Body)
	return err
}
