#!/bin/sh
./build.sh
ssh oysteinl@panda.localdomain sudo /usr/sbin/service tesla-client stop
scp ./tesla-client panda.localdomain:tesla-client/
ssh oysteinl@panda.localdomain sudo /usr/sbin/service tesla-client start

