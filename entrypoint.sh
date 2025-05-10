#!/bin/sh
crond -f -d 8 &
/main -load place.png -saveInterval 30
