#!/bin/bash

echo "================ Backup ==============="
cd /portainer/Files/AppData/Config/rPlace/
cp place.png bak/$(date '+%Y-%m-%d_%H-%M-%S').png
echo "Backup created"

echo "============== Timelapse =============="
./timelapse.sh
echo "Timelapse generated"
