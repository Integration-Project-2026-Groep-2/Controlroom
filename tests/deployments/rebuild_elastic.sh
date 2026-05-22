#!/bin/sh

docker compose down elasticsearch
docker compose up elasticsearch -d --force-recreate --remove-orphans --yes --build
