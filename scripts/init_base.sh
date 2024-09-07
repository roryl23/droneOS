#!/usr/bin/env bash

SSID=$(cat ssid)
PASSWORD=$(cat ssid_password)

# initialize wifi network
sudo nmcli device wifi hotspot ssid "$SSID" password "$PASSWORD"
