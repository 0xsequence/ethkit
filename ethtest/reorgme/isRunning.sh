#!/usr/bin/env bash

yarn chain:ip --index 0 > /dev/null 2>&1
RES=$?

if [ $RES == 1 ]
then
  echo "*****"
  echo "Oops! reorgme is not running. Please run 'make start-reorgme'."
  echo "*****"
  exit 1
fi

exit 0
