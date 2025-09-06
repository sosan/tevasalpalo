#!/bin/bash
cd ..
docker build --build-arg ENV=prod --progress=plain -t portable -f scripts/Dockerfile .