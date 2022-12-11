#!/bin/bash

# Run with `nohup ./run-lc.sh&` if not having systemd handle it as a service

sudo iw dev wlan0 set power_save off
sudo /home/pi/light-control &>>/home/pi/lc-logs
