#!/usr/bin/env bash

MODE=${1:-"base"}

./droneOS_"${MODE}" --mode "$MODE"
