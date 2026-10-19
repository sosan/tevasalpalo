#!/bin/bash
# docker run --rm -p 9050:9050 -e SOCKS_HOSTNAME=0.0.0.0 leplusorg/tor &
docker run -it --rm -p 9050:9050 -p 3000:3000 portable
