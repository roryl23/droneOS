#!/usr/bin/env bash

./build/droneOS/base.bin --config-file ./configs/config.yaml &
basePid=$!
./build/droneOS/drone.bin --config-file ./configs/config.yaml &
dronePid=$!

# define cleanup function
cleanup() {
  echo "terminating background processes..."
  kill "$basePid" "$dronePid"
  wait "$basePid" "$dronePid"
}

# trap EXIT and other signals
trap cleanup EXIT SIGINT SIGTERM

wait
