#!/bin/sh

docker compose down kibana
docker compose up kibana -d --force-recreate --remove-orphans --yes --build
