ffmpeg -y -framerate 5 -pattern_type glob -i 'bak/*.png' -c:v libx264 -pix_fmt yuv420p -vf "pad=width=512:height=256:color=black" web/root/timelapse.mp4
#ffmpeg -y -i web/root/timelapse.mp4 web/root/timelapse.webm