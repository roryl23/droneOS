#!/usr/bin/env bash

# install libonnx
if ! [ -d "libonnx" ]; then
  git clone git@github.com:xboot/libonnx.git
fi
