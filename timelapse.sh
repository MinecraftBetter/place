#!/bin/bash
cd /portainer/Files/AppData/Config/rPlace/ || exit

echo "============== Timelapse =============="
echo "=== Remove corrupted files ==="
find bak/$(date '+%Y-%m') -name $(date '+%Y-%m-%d')* -type f | xargs -I % sh -c 'identify -verbose % > /dev/null 2>&1; if [ $? -eq 1 ]; then mv % bak/corrupted/; fi'
find bak/$(date -d "yesterday 13:00" '+%Y-%m') -name $(date -d "yesterday 13:00" '+%Y-%m-%d')* -type f | xargs -I % sh -c 'identify -verbose % > /dev/null 2>&1; if [ $? -eq 1 ]; then mv % bak/corrupted/; fi'
echo "========== Archives =========="
for month in bak/*/; do
  echo "====== $(basename "$month") ======"
  if [ "$(ls -A "$month")" ]; then
    zip -jr "web/root/archives/archive-$(basename "$month").zip" "$month" -i "*.png"
  else
    echo "Skipping"
  fi
done
echo "========== Video =========="
echo "====== 1st pass ======"
cat bak/*/*.png | ffmpeg -framerate 30 -f image2pipe -i - -vf "pad=width=1024:height=768:color=black,format=yuv420p" -y bak/timelapse.mkv
echo "====== 2nd pass ======"
ffmpeg -i bak/timelapse.mkv -pix_fmt yuv420p -y web/root/timelapse.mp4
