#!/bin/bash

echo "================ Backup ==============="
mkdir -p "bak/$(date '+%Y-%m')"
rm bak/temp.png
cp place.png bak/temp.png
lastMd=$(md5sum "$(ls -t bak/*/*.png | head -n1)" | cut -d " " -f 1)
if [[ $(md5sum bak/temp.png | cut -d " " -f 1) != "$lastMd" ]]; then
	mv bak/temp.png "bak/$(date '+%Y-%m')/$(date '+%Y-%m-%d_%H-%M-%S').png"
	echo "Backup created"
fi

echo "============== Timelapse =============="
./timelapse.sh
echo "Timelapse generated"
