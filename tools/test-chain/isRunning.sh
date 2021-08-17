#!/usr/bin/env bash

yarn chain:ip --index 0 > /dev/null 2>&1
RES=$?

if [ $RES == 1 ]
then
  echo "*****"
  echo "Oops! test-chain is not running. Please run 'make start-test-chain' in another terminal or use 'test-skip-reorgme'."
  echo "*****"
  exit 1
fi

exit 0
