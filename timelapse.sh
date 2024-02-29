#!/bin/bash
echo "============== Timelapse =============="
echo "========== Archives =========="
for month in bak/*/; do
  echo "====== $(basename "$month") ======"
  zip -jr "web/root/archives/archive-$(basename "$month").zip" "$month" -i "*.png"
done
echo "========== Video =========="
ffmpeg -framerate 30 -f image2 -export_path_metadata 1 -pattern_type glob -i 'bak/*/*.png' -vf "pad=width=1024:height=768:color=black" -y bak/timelapse.mkv
ffmpeg -i bak/timelapse.mkv -pix_fmt yuv420p -y web/root/timelapse.mp4
