#!/bin/sh

docker compose down controlroom
docker compose up controlroom -d --force-recreate --remove-orphans --yes --build
