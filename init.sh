#!/usr/bin/env bash

./init

# shellcheck disable=SC2181
if [ $? != 0 ]; then
  echo 'init program fail ,will exit $?'
  exit 1
fi

exec /opt/bitnami/scripts/mariadb-galera/entrypoint.sh /opt/bitnami/scripts/mariadb-galera/run.sh
