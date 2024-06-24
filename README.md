# Reverse Proxy

## Description

This project implements a reverse proxy in Go (Golang) that provides several key functionalities for handling incoming HTTP/HTTPS/Websockets requests.

## Supported Functionality

The reverse proxy supports the following functionalities:

### Reverse Proxy Functionality

The reverse proxy forwards incoming requests to a pool of backend servers. It acts as an intermediary between clients and backend servers, allowing for load balancing. List of backends is configured via environment variables using prefix `HTTP_SERVER_URL_` and `HTTPS_SERVER_URL_` , e.g. `HTTP_SERVER_URL_backend1`, `HTTP_SERVER_URL_back2`.

### Authorization

The reverse proxy implements a basic authorization mechanism. It checks for a `X-Auth-Token` header in incoming requests. Only requests with a valid token are forwarded to the backend servers. Tokens are provided via environment variables using prefix `AUTH_TOKEN`, e.g `AUTH_TOKEN_1`, `AUTH_TOKEN_backend_2`

### WebSocket Support

The reverse proxy supports WebSocket connections. It correctly handles WebSocket upgrades and forwards WebSocket traffic to the backend servers. This allows for real-time communication between clients and servers.

### Graceful Shutdown

The reverse proxy implements a graceful shutdown mechanism. When a shutdown signal (e.g., SIGINT or SIGTERM) is received, the proxy performs the following steps:

1. Stops accepting new connections.
2. Waits for all ongoing HTTP requests to complete.
3. Maintains active WebSocket connections until they are closed by the client or backend server. 
4. Provides a reasonable timeout period for ongoing WebSocket connections to close before forcefully shutting down (20s by default).

Functionality related to graceful websockets shutdown is implemented by keeping track of active ws connections and waiting for them to complete or until timeout is reached.

## Testing
Some functionality is covered with unit tests. But the core features are covered with end to end tests.
E2e tests reside in `tests/test_ws_client_test.go` file. Tests directory also contains test web server (`test_server.go`) and a script to run and cleanup backend web servers (`run_backends.sh`)

All tests in `test_ws_client_test.go` require proxy to be running as well as the backend servers. **Everything is started automatically when any of the tests are running**.

`TestHttpRoundRobin` covers http proxying and round robin logic.<br> 
`TestHttpsRoundRobin` covers https proxying and round robin logic. <br> 
`TestWebsocketsRoundRobin` covers websockets proxying and round robin logic. <br> 
`TestGracefulShutdown` covers graceful shutdown of websockets connections. It initiates the WS connection, makes the server wait for some time, meanwhile SIGTERM is sent to the proxy. The test verifies that we still get response from the server even after termination attempt was made. If the wait time is longer than the timeout then the connection are terminated.

## Configuration

Configuration can be provided via environment variables or via dotenv file. You can find a sample in .env file

## Assumptions and Design Decisions

The project makes the following assumptions and design decisions:<br> 

It was not clear how much of the code I need to write by myself and how much can be delegated to  3-rd party libraries, so I implemented some of the high level parts manually while leaving low-level or error prone parts to the libraries.  <br> 

For http/https I used a built-in reverse proxy from the standard httputils package. <br> 

I assumed that it was important to write some actual proxy-related code and to show that I’ve done some research, so for websockets I implemented the proxy manually (except the upgrade functionality that I delegate to ‘gorilla/websocket’).<br> 

## Improvements

The bottleneck of this server will likely be Network I/O, CPU, memory allocations. 
Possible ways of improvement:
- consider using pool of objects if possible. Goroutine pool (`https://github.com/panjf2000/ants`), maybe pool of memory buffers when copying messages
- consider switching to faster http stack. Maybe try `https://github.com/valyala/fasthttp`
