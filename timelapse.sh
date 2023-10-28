#!/bin/bash
for month in bak/*/; do
  zip "web/root/archives/archive-$(basename "$month").zip" "$month/*.png"
done
ffmpeg -framerate 30 -f image2 -export_path_metadata 1 -pattern_type glob -i 'bak/*/*.png' -vf "pad=width=1024:height=768:color=black" -y bak/timelapse.mkv
ffmpeg -i bak/timelapse.mkv -y web/root/timelapse.mp4
