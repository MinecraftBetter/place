#!/bin/bash
echo "============== Timelapse =============="
echo "========== Archives =========="
for month in bak/*/; do
  echo "====== $(basename "$month") ======"
  zip -jr "web/root/archives/archive-$(basename "$month").zip" "$month" -i "*.png"
done
echo "========== Video =========="
echo "====== 1st pass ======"
cat bak/*/*.png | ffmpeg -framerate 30 -f image2pipe -i - -vf "pad=width=1024:height=768:color=black,format=yuv420p" -y bak/timelapse.mkv
echo "====== 2nd pass ======"
ffmpeg -i bak/timelapse.mkv -pix_fmt yuv420p -y web/root/timelapse.mp4
