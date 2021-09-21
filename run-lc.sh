#!/bin/bash

# Run with: `nohup ./run-lc.sh&`

fifo="/home/pi/fifo"

rm $fifo
mkfifo $fifo
cat /dev/input/event0 >>$fifo&
cat /dev/input/event1 >>$fifo&
/home/pi/light-control
