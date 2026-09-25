#!/bin/sh
set -e
sudo docker rm -f chr-lab 2>/dev/null || true
sudo docker run --name chr-lab --privileged --network mega_magic_dev_network \
  -h chr-lab mikrotik/chr:stable -it
