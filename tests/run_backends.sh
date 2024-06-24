#!/bin/bash

go build -o test_server test_server.go

start_server() {
  PORT=$1
  #Kill the process if it is already running
  lsof -ti:$PORT | xargs kill -9

  echo "Starting server on port $PORT"
  ./test_server $PORT &
}

start_server 8081
start_server 8082
start_server 8083

echo "Press [CTRL+C] to stop the servers."
wait
