#!/usr/bin/env bash

# install LibNC
if ! [ -d "libnc" ]; then
  if ! [ -f "libnc-2021-04-24.tar.gz" ]; then
    wget --no-check-certificate https://www.bellard.org/libnc/libnc-2021-04-24.tar.gz
  fi
  tar -xvf libnc-2021-04-24.tar.gz && \
  mv libnc-2021-04-24 libnc
fi

# install TinyCC
if ! [ -d "tcc" ]; then
  if ! [ -f "tcc-0.9.27.tar.bz2" ]; then
    wget --no-check-certificate http://download.savannah.gnu.org/releases/tinycc/tcc-0.9.27.tar.bz2
  fi
  tar -xvf tcc-0.9.27.tar.bz2 && \
  mv tcc-0.9.27 tcc
fi

# install libonnx
if ! [ -d "libonnx" ]; then
  git clone git@github.com:xboot/libonnx.git
fi
