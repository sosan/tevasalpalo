#!/bin/bash
cd ..
docker build --build-arg ENV=dev --progress=plain -t portable -f ./scripts/Dockerfile .
