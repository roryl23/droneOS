#!/usr/bin/env bash

# install Arduino libraries
if ! [ -d "ArduinoCore-avr" ]; then
  git clone git@github.com:arduino/ArduinoCore-avr.git
fi
if ! [ -d "Servo" ]; then
  git clone git@github.com:arduino-libraries/Servo.git
fi

# install libonnx
if ! [ -d "libonnx" ]; then
  git clone git@github.com:xboot/libonnx.git
fi
